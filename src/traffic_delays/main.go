package main

import (
	"context"
	"encoding/json"
	"fmt"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/matteoavallone7/optimaLDN/src/common"
	"github.com/matteoavallone7/optimaLDN/src/rabbitmq"
	"github.com/matteoavallone7/optimaLDN/src/traffic_delays/internal"
	amqp "github.com/rabbitmq/amqp091-go"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	traffic_exchange = "traffic_events_exchange"
	exchange_type    = "topic"
)

var influBucket string
var influDBUrl string
var influOrg string
var influDBToken string
var influClient influxdb2.Client

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func startDelayMonitor(ctx context.Context, newPublisher *rabbitmq.Publisher) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			log.Println("Running delay detection...")
			criticalDelaysQuery := fmt.Sprintf(`
		from(bucket: "%s")
		  |> range(start: -15m)
		  |> filter(fn: (r) => r._measurement == "tfl_line_status")
          |> filter(fn: (r) =>
            r.status_severity_description == "Severe Delays" or
            r.status_severity_description == "Part Suspended" or
            r.status_severity_description == "Closed"
          )
          |> group(columns: ["line_name", "mode_name"]) 
          |> last() // Get the latest status for each line that matches the criteria
		  |> keep(columns: ["_time", "line_name", "mode_name", "status_severity_description", "reason"])
	`, influBucket)

			criticalDelayMessages, err := internal.ExecuteAndProcessQuery(ctx, criticalDelaysQuery, "Critical Delay")
			if err != nil {
				failOnError(err, "Error processing critical delays")
			}

			suddenDropQuery := fmt.Sprintf(`
		from(bucket: "%s")
		  |> range(start: -30m)
		  |> filter(fn: (r) => r._measurement == "tfl_line_status" and r._field == "status_severity")
		  |> group(columns: ["line_name", "mode_name"])
		  |> sort(columns: ["_time"])
		  |> difference(columns: ["_value"])
		  |> filter(fn: (r) => r._value < -3.0) // Threshold: A drop of more than 3 points in severity
		  |> keep(columns: ["_time", "line_name", "mode_name", "_value"])
	`, influBucket)

			suddenDropMessages, err := internal.ExecuteAndProcessQuery(ctx, suddenDropQuery, "Sudden Service Worsening")
			if err != nil {
				failOnError(err, "Error processing sudden severity drops")
			}

			// --- Send Notifications to RabbitMQ based on type ---

			// Send Critical Delay messages if any
			if len(criticalDelayMessages) > 0 {
				payload := common.NotificationPayload{
					AlertType:   "CriticalDelay",
					Alerts:      criticalDelayMessages,
					GeneratedAt: time.Now(),
				}
				jsonBody, marshalErr := json.Marshal(payload)
				if marshalErr != nil {
					log.Printf("Failed to marshal CriticalDelay payload to JSON: %v", marshalErr)
				} else {
					headers := amqp.Table{"Alert-Type": "CriticalDelay"}
					publishErr := newPublisher.Publish("traffic.route.update.critical", jsonBody, headers)
					if publishErr != nil {
						log.Printf("Failed to publish Critical Delays RabbitMQ message: %v", publishErr)
					} else {
						log.Println("Critical Delays notification sent via RabbitMQ.")
					}
				}
			}

			// Send Sudden Service Worsening messages if any
			if len(suddenDropMessages) > 0 {
				payload := common.NotificationPayload{
					AlertType:   "SuddenServiceWorsening",
					Alerts:      suddenDropMessages,
					GeneratedAt: time.Now(),
				}
				data, marshalErr := json.Marshal(payload)
				if marshalErr != nil {
					log.Printf("Failed to marshal SuddenServiceWorsening payload to JSON: %v", marshalErr)
				} else {
					headers := amqp.Table{"Alert-Type": "SuddenServiceWorsening"}
					publishErr := newPublisher.Publish("traffic.route.update.sudden", data, headers)
					if publishErr != nil {
						log.Printf("Failed to publish Sudden Service Worsening RabbitMQ message: %v", publishErr)
					} else {
						log.Println("Sudden Service Worsening notification sent via RabbitMQ.")
					}
				}
			}

			if len(criticalDelayMessages) == 0 && len(suddenDropMessages) == 0 {
				log.Println("No significant TfL anomalies or negative trends detected at this time. All clear.")
			}

		case <-ctx.Done():
			log.Println("Delay monitor stopping due to context cancel.")
			return
		}
	}
}

func main() {
	fmt.Println("Starting Traffic_delays service...")

	influDBUrl = os.Getenv("INFLUXDB_URL")
	influOrg = os.Getenv("INFLUXDB_ORG")
	influBucket = os.Getenv("INFLUXDB_BUCKET")
	influDBToken = os.Getenv("INFLUXDB_TOKEN")

	if influDBUrl == "" || influOrg == "" || influBucket == "" || influDBToken == "" {
		log.Fatal("One or more InfluxDB environment variables (INFLUXDB_URL, INFLUXDB_ORG, INFLUXDB_BUCKET, INFLUXDB_TOKEN) not set. Please configure them in Lambda settings.")
	}

	if influClient == nil {
		influClient = influxdb2.NewClient(influDBUrl, influDBToken)
		internal.InfluxQueryAPI = influClient.QueryAPI(influOrg)
		log.Println("InfluxDB query client initialized.")
	}
	defer influClient.Close()

	fmt.Println("Setting up RabbitMQ...")
	conn, ch, err := rabbitmq.InitRabbitMQ(traffic_exchange, exchange_type)
	failOnError(err, "Failed to initialize RabbitMQ for Notification Service's outbound events")

	defer func() {
		if cleanupErr := rabbitmq.CloseResources(ch, conn)(); cleanupErr != nil {
			log.Printf("Error during RabbitMQ resource cleanup: %v", cleanupErr)
		}
	}()

	trafficPublisher := rabbitmq.NewPublisher(ch, traffic_exchange)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	wg.Add(1)
	go func() {
		defer wg.Done()
		startDelayMonitor(ctx, trafficPublisher)
	}()

	<-sigChan
	log.Println("Shutdown signal received.")
	cancel()

	wg.Wait()
	log.Println("Application shut down cleanly.")

}
