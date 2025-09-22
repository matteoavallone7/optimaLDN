package main

import (
	"encoding/json"
	"github.com/matteoavallone7/optimaLDN/src/common"
	"github.com/rabbitmq/amqp091-go"
	"log"
	"time"
)

func main() {
	rabbitURL := "amqp://guest:guest@ec2-3-80-26-146.compute-1.amazonaws.com:5672/"

	conn, err := amqp091.Dial(rabbitURL)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open channel: %v", err)
	}
	defer ch.Close()

	// Create a fake CriticalDelay alert
	payload := common.NotificationPayload{
		AlertType: "CriticalDelay",
		Alerts: []common.TfLAlert{
			{
				LineName:          "jubilee",
				ModeName:          "tube",
				Timestamp:         time.Now(),
				StatusDescription: "Severe Delays",
				Reason:            "Signal failure at Waterloo",
			},
		},
		GeneratedAt: time.Now(),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Fatalf("Failed to marshal payload: %v", err)
	}

	headers := amqp091.Table{"Alert-Type": "CriticalDelay"}

	err = ch.Publish(
		"traffic_events_exchange",
		"traffic.route.update.critical",
		false,
		false,
		amqp091.Publishing{
			ContentType: "application/json",
			Body:        body,
			Headers:     headers,
		},
	)
	if err != nil {
		log.Fatalf("Failed to publish message: %v", err)
	}

	log.Println("✅ Test CriticalDelay payload published to RabbitMQ.")
}
