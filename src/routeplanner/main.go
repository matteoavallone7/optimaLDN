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

func (r *RoutePlanner) ServeRequest(args *common.UserRequest, reply *common.RouteResult) error {

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
	journey, err := FetchRoutes(startPoint, endPoint, args.DepartureDate, args.DepartureTime)
	if err != nil {
		log.Printf("Error getting journey from start point '%s' to end point '%s'\n", startPoint, endPoint)
		return err
	}

	var bestScore = math.MaxFloat64
	var bestJourney common.TFLJourney

	for _, route := range journey.Journeys {
		currentScore := handleCrowding(route, args.DepartureDate)
		if currentScore < bestScore {
			bestScore = currentScore
			bestJourney = route
		}
	}

	reply.From = args.StartPoint
	reply.To = args.EndPoint
	reply.Score = bestScore
	reply.Summary = buildSummary(bestJourney)

	if okNotif := notifyNewRoute(args.UserID, bestJourney); okNotif != nil {
		return fmt.Errorf("failed to publish new active route: %w", err)
	}

	log.Println("New active route notification published successfully.")

	return nil
}

func (r *RoutePlanner) RecalculateRoute(args *common.NewRequest, reply *common.RouteResult) error {

	fmt.Printf("Request received from '%s' to '%s'\n", args.UserID, args.Reason)
	// prendi dati rotta attuale per creare active route da mandare a notif (non data che sta sotto!!)
	// avvisa notif service che la rotta non è piu valida
	data, _ := json.Marshal(args)
	if err := PublishMsg(routeTerminated, data); err != nil {
		return fmt.Errorf("failed to publish terminated route: %w", err)
	}
	log.Println("Terminated route notification published successfully.")

	// prendi da dynamo db la rotta corrente, calcola posizione approssimativa e ricalcola nuova rotta
	return nil
}

func (r *RoutePlanner) TerminateRoute(args *common.NewRequest, reply *common.SavedResp) error {

	fmt.Printf("Request received from '%s' to '%s'\n", args.UserID, args.Reason)
	data, _ := json.Marshal(args)
	if err := PublishMsg(routeTerminated, data); err != nil {
		return fmt.Errorf("failed to publish terminated route: %w", err)
	}
	log.Println("Terminated route notification published successfully.")

	// cancella da dynamodb rotta
	return nil
}

func (r *RoutePlanner) AcceptSavedRouteRequest(args *common.UserSavedRoute, reply *common.SavedResp) error {

	fmt.Printf("Requested saved route from '%s' to '%s'\n", args.StartPoint, args.EndPoint)
	fmt.Println("Registering route...")
	var activeRoute = common.ActiveRoute{
		args.RouteID,
		args.LineNames,
	}

	// rabbit al Notification

	reply.UserID = activeRoute.UserID
	reply.Status = common.StatusDone

	data, _ := json.Marshal(activeRoute)
	if err := PublishMsg(routeCreated, data); err != nil {
		return fmt.Errorf("failed to publish saved active route: %w", err)
	}

	log.Println("Saved active route notification published successfully.")

	return nil
}

func notifyNewRoute(user string, journey common.TFLJourney) error {

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
		return err
	}

	return nil
}

func buildSummary(journey common.TFLJourney) string {
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

	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}
	dbClient = dynamodb.NewFromConfig(cfg)
	log.Println("DynamoDB client initialized.")

	err = godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	tflAPIKey = os.Getenv("TFL_API_KEY")

	routePlanner := new(RoutePlanner)
	server := rpc.NewServer()
	err = server.Register(routePlanner)
	if err != nil {
		log.Fatalf("RPC error registering route planner: %v", err)
	}
	log.Println("Route planner service registered successfully.")

	port := os.Getenv("RP_PORT")
	if port == "" {
		log.Fatal("RP_PORT env variable not set")
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%s", port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	log.Printf("Listening on 0.0.0.0:%s", port)

	_, err = loadNapTanFile("stationCodes.csv")
	if err != nil {
		fmt.Println("Error while opening CSV file:", err)
		return
	}

	conn, ch, err := rabbitmq.InitRabbitMQ(routeOutboundNotifications, routeExchangeType)
	failOnError(err, "Failed to connect to RabbitMQ")

	defer func() {
		if cleanupErr := rabbitmq.CloseResources(ch, conn)(); cleanupErr != nil {
			log.Printf("Error during RabbitMQ resource cleanup: %v", cleanupErr)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup

	_, err = rabbitmq.DeclareAndBindQueue(ch, notificationQueueName, bindingKey, notificationOutboundExchangeName)
	failOnError(err, fmt.Sprintf("Failed to declare and bind queue '%s' for Notification Service", notificationQueueName))

	routePublisher = rabbitmq.NewPublisher(ch, routeOutboundNotifications)

	notifHandler := func(delivery amqp.Delivery) bool {

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
		}
	}()

	<-sigChan
	log.Println("Shutdown signal received. Stopping consumers...")

	cancel()

	wg.Wait()
	log.Println("Route planner service shut down cleanly.")

}
