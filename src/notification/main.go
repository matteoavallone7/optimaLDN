package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/patrickmn/go-cache"
	amqp "github.com/rabbitmq/amqp091-go"
	"log"
	"optimaLDN/src/common"
	"optimaLDN/src/rabbitmq"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	notificationOutboundExchangeName = "notification_outbound_events_exchange"
	notificationOutboundExchangeType = "topic"
	trafficQueueName                 = "traffic_queue"
	trafficBindingKey                = "traffic.route.update.#"
	trafficExchange                  = "traffic_events_exchange"
	routeQueueName                   = "route_panner_queue"
	routeBindingKey                  = "route.update.#"
	routeExchange                    = "route_planner_exchange"
	defaultCacheExpiration           = 5 * time.Minute // How long an item stays in cache
	cacheCleanupInterval             = 10 * time.Minute
)

var notificationPublisher *rabbitmq.Publisher
var dbClient *dynamodb.Client
var appCache *cache.Cache
var ctx context.Context

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func handleCriticalDelay(payload common.NotificationPayload) {
	for _, alert := range payload.Alerts {
		res, userIDs := CheckActiveRoutes(ctx, alert.LineName)
		if res {
			for _, userID := range userIDs {
				msg := fmt.Sprintf("Line %s for user %s is experiencing critical delays.", alert.LineName, userID)
				req := common.NewRequest{
					userID,
					msg,
				}
				data, err := json.Marshal(req)
				failOnError(err, "Failed to marshal critical delay json")
				err = notificationPublisher.Publish("route.update.critical", data, amqp.Table{"Alert-Type": payload.AlertType})
				failOnError(err, "Failed to publish notification")
			}
		} else {
			log.Printf("No active route subscriptions found for line '%s'. Skipping notification.", alert.LineName)
			continue
		}
	}
}

func handleSuddenDelay(payload common.NotificationPayload) {
	for _, alert := range payload.Alerts {
		res, userIDs := CheckActiveRoutes(alert.LineName)
		if res {
			for _, userID := range userIDs {
				msg := fmt.Sprintf("Line %s for user %s is experiencing sudden worsening delays.", alert.LineName, userID)
				req := common.NewRequest{
					UserID: userID,
					Reason: msg,
				}
				data, err := json.Marshal(req)
				failOnError(err, "Failed to marshal sudden delay JSON")
				err = notificationPublisher.Publish("user.update.sudden", data, amqp.Table{"Alert-Type": payload.AlertType})
				failOnError(err, "Failed to publish sudden delay notification")
			}
		} else {
			log.Printf("No active route subscriptions found for line '%s'. Skipping notification.", alert.LineName)
			continue
		}
	}
}

func main() {
	fmt.Println("Starting Notification service...")
	fmt.Println("Setting up RabbitMQ...")

	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}
	dbClient = dynamodb.NewFromConfig(cfg)
	log.Println("DynamoDB client initialized.")

	conn, ch, err := rabbitmq.InitRabbitMQ(notificationOutboundExchangeName, notificationOutboundExchangeType)
	failOnError(err, "Failed to initialize RabbitMQ for Notification Service's outbound events")

	defer func() {
		if cleanupErr := rabbitmq.CloseResources(ch, conn)(); cleanupErr != nil {
			log.Printf("Error during RabbitMQ resource cleanup: %v", cleanupErr)
		}
	}()

	notificationPublisher = rabbitmq.NewPublisher(ch, notificationOutboundExchangeName)

	_, err = rabbitmq.DeclareAndBindQueue(ch, trafficQueueName, trafficBindingKey, trafficExchange)
	failOnError(err, fmt.Sprintf("Failed to declare and bind queue '%s' for Traffic Service", trafficQueueName))

	appCache = cache.New(defaultCacheExpiration, cacheCleanupInterval)
	log.Println("In-memory cache initialized.")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup

	trafficHandler := func(delivery amqp.Delivery) bool {
		log.Printf("[Notification Service] Received Delay Event: %s (Key: %s)", string(delivery.Body), delivery.RoutingKey)
		var payload common.NotificationPayload
		err := json.Unmarshal(delivery.Body, &payload)
		failOnError(err, "Failed to unmarshal notification payload")
		log.Printf("[Notification Service] Received Payload: %s, Alerts: %s, Generated: %s", payload.AlertType, len(payload.Alerts), payload.GeneratedAt)
		switch payload.AlertType {
		case "CriticalDelay":
			handleCriticalDelay(payload)
		case "SuddenServiceWorsening":
			handleSuddenDelay(payload)
		default:
			log.Printf("  -> Unrecognized alert type in payload: '%s'.", payload.AlertType)
		}

		return true
	}
	trafficConsumer := rabbitmq.NewConsumer(ch, trafficQueueName, trafficHandler)
	wg.Add(1)
	go func() {
		defer wg.Done()
		trafficConsumer.StartConsume(ctx)
	}()

	_, err = rabbitmq.DeclareAndBindQueue(ch, routeQueueName, routeBindingKey, routeExchange)
	failOnError(err, fmt.Sprintf("Failed to declare and bind queue '%s' for Route Planner Service", routeQueueName))

	routePlannerHandler := func(delivery amqp.Delivery) bool {
		log.Printf("[Notification Service] Received Route Planner Event: %s (Key: %s)", string(delivery.Body), delivery.RoutingKey)

		var req common.ActiveRoute
		err = json.Unmarshal(delivery.Body, &req)
		if err != nil {
			log.Printf("Failed to unmarshal route planner payload: %v", err)
			return false
		}

		switch delivery.RoutingKey {
		case "active.route.created":
			err = RegisterNewRoute(req)
			if err != nil {
				log.Printf("Failed to write active route for user %s: %v", req.UserID, err)
				return false
			}
			log.Printf("Stored active route for user %s.", req.UserID)

		case "active.route.terminated":
			deletedRoute, err := DeleteActiveRoute(req)
			if err != nil {
				log.Printf("Failed to delete active route for user %s: %v", req.UserID, err)
				return false
			}
			if deletedRoute != nil {
				cacheKey := fmt.Sprintf("%s", deletedRoute.LineIDs)
				appCache.Delete(cacheKey)
				log.Printf("Deleted cache entry for key %s", cacheKey)
			}
			log.Printf("Deleted active route for user %s.", req.UserID)

		default:
			log.Printf("Unrecognized routing key: %s", delivery.RoutingKey)
			return false
		}

		return true
	}

	routePlannerConsumer := rabbitmq.NewConsumer(ch, routeQueueName, routePlannerHandler)
	wg.Add(1)
	go func() {
		defer wg.Done()
		routePlannerConsumer.StartConsume(ctx)
	}()

	<-sigChan
	log.Println("Shutdown signal received. Stopping consumers...")

	cancel()

	wg.Wait()
	log.Println("Notification service shut down cleanly.")

}
