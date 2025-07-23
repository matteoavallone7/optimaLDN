package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sony/gobreaker"
	"log"
	"time"
)

// Publisher represents a RabbitMQ message publisher with a circuit breaker.
type Publisher struct {
	channel  *amqp.Channel
	exchange string
	cb       *gobreaker.CircuitBreaker
}

// NewPublisher creates a new Publisher instance.
// It initializes a circuit breaker for publishing operations.
//
// Parameters:
//
//	ch: The active RabbitMQ channel.
//	exchangeName: The name of the exchange to publish to.
//	routingKey: The default routing key for messages from this publisher.
//
// Returns:
//
//	*Publisher: A new Publisher instance.
func NewPublisher(ch *amqp.Channel, exchangeName string) *Publisher {
	// Configure the circuit breaker for publishing operations.
	// Adjust these parameters based on your application's requirements.
	settings := gobreaker.Settings{
		Name:        "RabbitMQPublisher",
		MaxRequests: 3,                // The maximum number of requests allowed to pass through when the circuit is half-open.
		Interval:    30 * time.Second, // The period of the circuit breaker's closed state.
		Timeout:     10 * time.Second, // The period of the circuit breaker's open state.
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Trip the circuit if there are more than 3 consecutive failures.
			return counts.ConsecutiveFailures > 3
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			log.Printf("Circuit breaker %s changed state from %s to %s\n", name, from.String(), to.String())
		},
	}
	cb := gobreaker.NewCircuitBreaker(settings)

	return &Publisher{
		channel:  ch,
		exchange: exchangeName,
		cb:       cb,
	}
}

// Publish sends a message to the configured RabbitMQ exchange using the circuit breaker.
// It attempts to publish once per circuit breaker execution.
// If the circuit breaker is open, it will return a gobreaker.ErrOpenState error.
//
// Parameters:
//
//	routingKey: The specific routing key for this message.
//	body: The message payload as a byte slice.
//
// Returns:
//
//	error: An error if the publish operation fails (including circuit breaker errors).
func (p *Publisher) Publish(routingKey string, body []byte, headers amqp.Table) error {
	// Execute the publish operation through the circuit breaker.
	// The circuit breaker's `Execute` method will call the provided function.
	_, err := p.cb.Execute(func() (interface{}, error) {
		// Use a context with a timeout for the publish operation itself.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		// Publish the message.
		// mandatory: false -> if message cannot be routed, it's dropped (true would return it to publisher via `Return` channel)
		// immediate: false -> not used in modern RabbitMQ, but kept for compatibility
		err := p.channel.PublishWithContext(
			ctx,
			p.exchange, // exchange name
			routingKey, // routing key
			false,      // mandatory
			false,      // immediate
			amqp.Publishing{
				ContentType:  "application/json",
				Body:         body,
				Headers:      headers,
				DeliveryMode: amqp.Persistent, // Make messages persistent so they survive broker restarts
			},
		)
		if err != nil {
			// Log the specific publish error, but let the circuit breaker handle retries/state changes.
			log.Printf("Actual publish attempt failed: %v", err)
			return nil, err // Return error to the circuit breaker
		}
		return nil, nil // Success
	})

	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) {
			log.Println("Circuit breaker is OPEN for publisher — skipping publish attempt.")
		} else {
			log.Printf("Publish operation failed via circuit breaker: %v", err)
		}
		return fmt.Errorf("publish operation failed: %w", err)
	}

	log.Printf("Message published successfully to exchange '%s' with routing key '%s'", p.exchange, routingKey)
	return nil
}
