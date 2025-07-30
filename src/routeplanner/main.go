package routeplanner

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/joho/godotenv"
	"github.com/matteoavallone7/optimaLDN/src/common"
	"github.com/matteoavallone7/optimaLDN/src/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
	"log"
	"math"
	"net"
	"net/rpc"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type RoutePlanner struct{}

var routePublisher *rabbitmq.Publisher
var dbClient *dynamodb.Client
var tflAPIKey string
var NaptanMap = make(map[string]string)

const (
	routeCreated                     = "active.route.created"
	routeTerminated                  = "active.route.terminated"
	routeOutboundNotifications       = "route_planner_exchange"
	notificationOutboundExchangeName = "notification_outbound_events_exchange"
	routeExchangeType                = "topic"
	notificationQueueName            = "notifications_queue"
	bindingKey                       = "route.update.#"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func (r *RoutePlanner) ServeRequest(ctx context.Context, args *common.UserRequest, reply *common.RouteResult) error {

	fmt.Printf("Requested route from '%s' to '%s'\n", args.StartPoint, args.EndPoint)
	fmt.Println("Acquiring start-point NapTan code..")
	startPoint, okStart := GetNaptan(args.StartPoint)
	if !okStart {
		return fmt.Errorf("start point '%s' not found in station mapping", args.StartPoint)
	}

	fmt.Println("Acquiring end-point NapTan code..")
	endPoint, okEnd := GetNaptan(args.EndPoint)
	if !okEnd {
		return fmt.Errorf("end point '%s' not found in station mapping", args.EndPoint)
	}

	fmt.Println("Fetching available routes..")
	bestJourney, bestScore, err := findBestJourney(startPoint, endPoint, args.Departure)
	if err != nil {
		return fmt.Errorf("could not find best journey: %s", err)
	}

	reply.From = args.StartPoint
	reply.To = args.EndPoint
	reply.Score = bestScore
	reply.Summary = buildSummary(bestJourney)

	if okNotif := notifyNewRoute(args.UserID, bestJourney); okNotif != nil {
		return fmt.Errorf("failed to publish new active route: %w", err)
	}
	log.Println("New active route notification published successfully.")

	chosen := ConvertToChosenRoute(args.UserID, *bestJourney)
	if err = SaveChosenRoute(ctx, chosen); err != nil {
		log.Printf("Error saving chosen route: %v", err)
	}
	return nil
}

func (r *RoutePlanner) GetCurrentRoute(ctx context.Context, args *common.NewRequest, reply *common.ChosenRoute) error {
	route, err := GetActiveRoute(ctx, args.UserID)
	if err != nil {
		return err
	}
	*reply = *route
	return nil
}

func findBestJourney(startNaptan, endNaptan string, departure time.Time) (*common.TFLJourney, float64, error) {
	journeys, err := FetchRoutes(startNaptan, endNaptan, departure)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch routes: %w", err)
	}

	var bestScore = math.MaxFloat64
	var bestJourney *common.TFLJourney

	for _, route := range journeys.Journeys {
		currentScore := handleCrowding(route, departure.Format("20060102"))
		if currentScore < bestScore {
			bestScore = currentScore
			bestJourney = &route
		}
	}

	if bestJourney == nil {
		return nil, 0, fmt.Errorf("no valid journeys found")
	}

	return bestJourney, bestScore, nil
}

func (r *RoutePlanner) RecalculateRoute(ctx context.Context, args *common.NewRequest, reply *common.RouteResult) error {

	fmt.Printf("Request received from '%s' to '%s'\n", args.UserID, args.Reason)
	chosenRoute, err := sharedRecalculationLogic(ctx, args.UserID)
	if err != nil {
		return err
	}

	currentStop, err1 := EstimateCurrentStop(*chosenRoute)
	if err1 != nil {
		return err1
	}

	currentStopNaptan, ok := GetNaptan(currentStop)
	if !ok {
		return fmt.Errorf("could not find Naptan for current stop point")
	}

	lastLeg := chosenRoute.Legs[len(chosenRoute.Legs)-1]
	endPoint := lastLeg.ToID
	bestJourney, bestScore, err2 := findBestJourney(currentStopNaptan, endPoint, time.Now())
	if err2 != nil {
		return fmt.Errorf("failed to find best recalculation journey: %w", err2)
	}

	if okNotif := notifyNewRoute(args.UserID, bestJourney); okNotif != nil {
		return fmt.Errorf("failed to publish new active route: %w", err)
	}
	log.Println("New active route notification published successfully.")

	if err = SaveChosenRoute(ctx, *chosenRoute); err != nil {
		log.Printf("Error saving chosen route: %v", err)
	}

	reply.From = chosenRoute.Legs[0].From
	reply.To = lastLeg.To
	reply.Score = bestScore
	reply.Summary = buildSummary(bestJourney)

	return nil
}

func sharedRecalculationLogic(ctx context.Context, userID string) (*common.ChosenRoute, error) {
	chosenRoute, err := GetActiveRoute(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active route for user %s: %w", userID, err)
	}
	activeRoute := ConvertToActiveRoute(userID, chosenRoute)
	data, _ := json.Marshal(activeRoute)
	if err = routePublisher.Publish(routeTerminated, data, amqp.Table{"Event-Type": "Route Aborted"}); err != nil {
		return nil, fmt.Errorf("no active route found for user %s", userID)
	}
	log.Println("Aborted route notification published successfully.")

	return chosenRoute, nil
}

func (r *RoutePlanner) TerminateRoute(ctx context.Context, args *common.NewRequest, reply *common.SavedResp) error {

	fmt.Printf("Request received from '%s' to '%s'\n", args.UserID, args.Reason)
	activeRoute, err := GetActiveRoute(ctx, args.UserID)
	if err != nil {
		return fmt.Errorf("failed to get active route for user %s: %w", args.UserID, err)
	}
	data, _ := json.Marshal(activeRoute)
	if err := routePublisher.Publish(routeTerminated, data, amqp.Table{"Event-Type": "Route Terminated"}); err != nil {
		return fmt.Errorf("failed to publish terminated route: %w", err)
	}
	log.Println("Terminated route notification published successfully.")

	err = DeleteChosenRoute(ctx, args.UserID)
	if err != nil {
		return fmt.Errorf("failed to delete chosen route: %w", err)
	}

	reply.UserID = args.UserID
	reply.Status = common.StatusDone

	return nil
}

func (r *RoutePlanner) AcceptSavedRouteRequest(args *common.UserSavedRoute, reply *common.SavedResp) error {

	fmt.Printf("Requested saved route from '%s' to '%s'\n", args.StartPoint, args.EndPoint)
	fmt.Println("Registering route...")
	var activeRoute = common.ActiveRoute{
		UserID:  args.RouteID,
		LineIDs: args.LineNames,
	}

	data, _ := json.Marshal(activeRoute)
	if err := routePublisher.Publish(routeCreated, data, amqp.Table{"Event-Type": "Route Created"}); err != nil {
		return fmt.Errorf("failed to publish saved active route: %w", err)
	}

	log.Println("Saved active route notification published successfully.")
	reply.UserID = activeRoute.UserID
	reply.Status = common.StatusDone

	return nil
}

func notifyNewRoute(user string, journey *common.TFLJourney) error {

	var lines []string
	var newRoute common.ActiveRoute

	for _, leg := range journey.Legs {
		for _, option := range leg.RouteOptions {
			lines = append(lines, option.LineIdentifier.ID)
		}

	}
	newRoute.UserID = user
	newRoute.LineIDs = lines
	data, _ := json.Marshal(newRoute)

	if err := routePublisher.Publish(routeCreated, data, amqp.Table{"Event-Type": "Route Created"}); err != nil {
		return fmt.Errorf("failed to publish new active route: %w", err)
	}

	return nil
}

func buildSummary(journey *common.TFLJourney) string {
	var summary []string

	for _, leg := range journey.Legs {
		summary = append(summary, leg.Instruction.Detailed)
	}

	return strings.Join(summary, " → ")
}

func handleCrowding(journey common.TFLJourney, day string) float64 {
	var totalCrowding float64
	var stopCount int

	dayOfWeek := findDayOfWeek(day)
	for _, leg := range journey.Legs {
		timeBand, err := TimeStringToTfLTimeBand(leg.ArrivalTime)
		if err != nil {
			log.Printf("Invalid arrival time %s: %v", leg.ArrivalTime, err)
			continue
		}
		for _, stop := range leg.Path.StopPoints {
			crowd, erro := FetchCrowding(stop.ID, dayOfWeek)
			if erro != nil {
				log.Printf("Could not fetch crowding for stop %s: %v", stop.ID, err)
				continue
			}
			for _, tb := range crowd.TimeBands {
				if timeBand == tb.TimeBand {
					totalCrowding += tb.PercentageOfBaseLine
					stopCount++
					break
				}
			}
		}
	}

	score := GetScore(totalCrowding, stopCount, journey.Duration)
	return score
}

func findDayOfWeek(day string) string {
	parsedDay, err := time.Parse("20060102", day)
	if err != nil {
		panic(err)
	}
	shortDay := parsedDay.Weekday().String()[:3]
	return strings.ToUpper(shortDay)
}

func loadNapTanFile(filename string) (map[string]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	for i, record := range records {
		if i == 0 {
			continue // skip header
		}
		if len(record) < 5 {
			continue // ensure valid row
		}
		commonName := record[1]
		naptanCode := record[4]
		NaptanMap[commonName] = naptanCode
	}

	return NaptanMap, nil
}

func main() {
	fmt.Println("Starting Route planner service...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx)
	failOnError(err, "Failed to load AWS config")
	dbClient = dynamodb.NewFromConfig(cfg)
	log.Println("DynamoDB client initialized.")

	err = godotenv.Load()
	failOnError(err, "Error loading .env file")

	tflAPIKey = os.Getenv("TFL_API_KEY")

	routePlanner := new(RoutePlanner)
	server := rpc.NewServer()
	err = server.Register(routePlanner)
	failOnError(err, "Failed to register route planner")
	log.Println("Route planner service registered successfully.")

	port := os.Getenv("RP_PORT")
	if port == "" {
		log.Fatal("RP_PORT env variable not set")
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%s", port))
	failOnError(err, "Failed to listen to TCP port")
	log.Printf("Listening on 0.0.0.0:%s", port)

	_, err = loadNapTanFile("stationCodes.csv")
	failOnError(err, "Failed to load stationCodes.csv")

	conn, ch, err := rabbitmq.InitRabbitMQ(routeOutboundNotifications, routeExchangeType)
	failOnError(err, "Failed to connect to RabbitMQ")

	defer func() {
		if cleanupErr := rabbitmq.CloseResources(ch, conn)(); cleanupErr != nil {
			log.Printf("Error during RabbitMQ resource cleanup: %v", cleanupErr)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup

	_, err = rabbitmq.DeclareAndBindQueue(ch, notificationQueueName, bindingKey, notificationOutboundExchangeName)
	failOnError(err, fmt.Sprintf("Failed to declare and bind queue '%s' for Notification Service", notificationQueueName))

	routePublisher = rabbitmq.NewPublisher(ch, routeOutboundNotifications)

	notifHandler := func(delivery amqp.Delivery) bool {
		log.Printf("[Route Planner Service] Received Delay Event: %s (Key: %s)", string(delivery.Body), delivery.RoutingKey)
		var payload common.NewRequest
		err2 := json.Unmarshal(delivery.Body, &payload)
		if err2 != nil {
			log.Printf("[Route Planner Service] Failed to unmarshal New Request: %v", err2)
			return false
		}
		route, err3 := sharedRecalculationLogic(ctx, payload.UserID)
		if err3 != nil {
			log.Printf("[Route Planner Service] Failed to calculate New Request: %v", err3)
			return false
		}
		currentStop, er := EstimateCurrentStop(*route)
		if er != nil {
			log.Printf("[Route Planner Service] Failed to estimate Current Stop: %v", er)
			return false
		}
		currentStopNaptan, ok := GetNaptan(currentStop)
		if !ok {
			log.Printf("[Route Planner Service] Failed to get Naptan: %v", currentStop)
			return false
		}
		if len(route.Legs) == 0 {
			log.Println("No legs in route")
			return false
		}
		lastLeg := route.Legs[len(route.Legs)-1]
		endPoint := lastLeg.ToID
		bestJourney, _, err2 := findBestJourney(currentStopNaptan, endPoint, time.Now())
		if err2 != nil {
			log.Printf("Failed to find best journey: %v", err2)
			return false
		}

		if okNotif := notifyNewRoute(payload.UserID, bestJourney); okNotif != nil {
			log.Printf("[Route Planner Service] Failed to notify route: %v", okNotif)
			return false
		}
		log.Println("New active route notification published successfully.")

		if err = SaveChosenRoute(ctx, *route); err != nil {
			log.Printf("Error saving chosen route: %v", err)
			return false
		}

		return true
	}

	routeConsumer := rabbitmq.NewConsumer(ch, notificationQueueName, notifHandler)
	wg.Add(1)
	go func() {
		defer wg.Done()
		routeConsumer.StartConsume(ctx)
	}()

	go func() {
		for {
			server.Accept(listener)
			if err != nil {
				log.Printf("RPC Accept error: %v", err)
				break // or continue, depending on if you want to retry
			}
		}
	}()

	<-sigChan
	log.Println("Shutdown signal received. Stopping consumers...")

	cancel()

	wg.Wait()
	log.Println("Route planner service shut down cleanly.")

}
