package routeplanner

import (
	"context"
	"errors"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sony/gobreaker"
)

const (
	rabbitMQURL  = "amqp://guest:guest@localhost:5672/"
	exchangeName = "route_events"
	exchangeType = "topic"
)

var breaker *gobreaker.CircuitBreaker

func failOnError(err error, msg string) {
	if err != nil {
		log.Panicf("%s: %s", msg, err)
	}
}
func exponentialBackoff(attempt int) time.Duration {
	base := time.Millisecond * 500
	return base * (1 << attempt) // 500ms, 1s, 2s, 4s, etc.
}

func PublishMsg(routingKey string, body []byte) error {

	var lastErr error

	// Execute the publish operation using the circuit breaker
	_, err := breaker.Execute(func() (interface{}, error) {
		for attempt := 0; attempt < 3; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			err := channel.PublishWithContext(
				ctx,
				exchangeName,
				routingKey,
				false,
				false,
				amqp.Publishing{
					ContentType: "application/json",
					Body:        body,
				},
			)
			if err == nil {
				return nil, nil // success
			}

			lastErr = err
			log.Printf("Publish failed (attempt %d): %v", attempt+1, err)

			// Exponential backoff before retrying
			time.Sleep(exponentialBackoff(attempt))
		}
		return nil, lastErr
	})

	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) {
			log.Println("Circuit breaker is OPEN — skipping publish attempt.")
		} else {
			log.Printf("Publish failed through circuit breaker: %v", err)
		}
		return err
	}

	return nil
}

func ConnectAmqp() (*amqp.Channel, func() error) {
	conn, err := amqp.Dial(rabbitMQURL)
	failOnError(err, "Failed to connect to RabbitMQ")

	ch, erro := conn.Channel()
	failOnError(erro, "Failed to open a channel")

	settings := gobreaker.Settings{
		Name:        "RabbitMQPublisher",
		MaxRequests: 1,                // Allowed requests in half-open state
		Interval:    30 * time.Second, // Rolling window
		Timeout:     10 * time.Second, // Time to retry after open
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures > 3
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			log.Printf("Circuit breaker %s changed from %s to %s\n", name, from.String(), to.String())
		},
	}
	breaker = gobreaker.NewCircuitBreaker(settings)

	err = ch.ExchangeDeclare(
		exchangeName, // name
		exchangeType, // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	failOnError(err, "Failed to declare an exchange")

	clean := func() error {
		errCh := ch.Close()
		errConn := conn.Close()
		if errCh != nil {
			return errCh
		}
		return errConn
	}

	return ch, clean
}
