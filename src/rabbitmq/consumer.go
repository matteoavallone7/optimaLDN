package rabbitmq

import (
	"context"
	"errors"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sony/gobreaker"
	"log"
	"time"
)

// MessageHandler defines the signature for a function that processes received messages.
// It should return true if the message was processed successfully and can be acknowledged,
// or false if processing failed and the message should be nacked (potentially re-queued or dead-lettered).
type MessageHandler func(delivery amqp.Delivery) bool

// Consumer represents a RabbitMQ message consumer with a circuit breaker.
type Consumer struct {
	channel        *amqp.Channel
	queueName      string
	messageHandler MessageHandler
	cb             *gobreaker.CircuitBreaker
	cbTimeout      time.Duration
}

// NewConsumer creates a new Consumer instance.
// It initializes a circuit breaker for consumption operations.
//
// Parameters:
//
//	ch: The active RabbitMQ channel.
//	queueName: The name of the queue to consume from.
//	handler: The function to call for each received message.
//
// Returns:
//
//	*Consumer: A new Consumer instance.
func NewConsumer(ch *amqp.Channel, queueName string, handler MessageHandler) *Consumer {
	// Configure the circuit breaker for consuming operations.
	settings := gobreaker.Settings{
		Name:        "RabbitMQConsumer",
		MaxRequests: 3,                // Max requests allowed in half-open state
		Interval:    30 * time.Second, // Period of the closed state
		Timeout:     10 * time.Second, // Period of the open state
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Trip if there are more than 3 consecutive failures in consumer registration/loop.
			return counts.ConsecutiveFailures > 3
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			log.Printf("Circuit breaker %s changed state from %s to %s\n", name, from.String(), to.String())
		},
	}
	cb := gobreaker.NewCircuitBreaker(settings)

	return &Consumer{
		channel:        ch,
		queueName:      queueName,
		messageHandler: handler,
		cb:             cb,
		cbTimeout:      settings.Timeout,
	}
}

// StartConsume begins consuming messages from the configured queue.
// It uses a circuit breaker to manage the underlying consumption process (e.g., channel issues).
// This function is blocking and should typically be run in a goroutine.
func (c *Consumer) StartConsume(ctx context.Context) {
	log.Printf("Consumer starting to listen on queue: %s", c.queueName)

	for {
		select {
		case <-ctx.Done():
			log.Println("Consumer received shutdown signal. Exiting consume loop.")
			return // Exit the StartConsume method

		default:
			// Execute the consume operation through the circuit breaker.
			_, err := c.cb.Execute(func() (interface{}, error) {
				// Register a consumer.
				msgs, err := c.channel.Consume(
					c.queueName, // queue name
					"",          // consumer tag
					false,       // auto-ack
					false,       // exclusive
					false,       // no-local
					false,       // no-wait
					nil,         // args
				)
				if err != nil {
					log.Printf("Failed to register a consumer: %v", err)
					return nil, err
				}

				// Loop indefinitely to process messages from the channel.
				for {
					select {
					case <-ctx.Done():
						log.Println("Context cancelled during message processing. Stopping inner consume loop.")
						// Return context.Canceled error to propagate shutdown signal through circuit breaker
						return nil, context.Canceled

					case d, ok := <-msgs:
						if !ok {
							log.Println("Consumer channel closed unexpectedly during message reception. Re-establishing consumer.")
							return nil, errors.New("consumer channel closed unexpectedly")
						}
						log.Printf("Received message from queue '%s': %s", c.queueName, string(d.Body))
						if c.messageHandler != nil {
							if c.messageHandler(d) {
								d.Ack(false)
								log.Println("Message acknowledged.")
							} else {
								d.Nack(false, true) // Nack and requeue
								log.Println("Message nacked and requeued.")
							}
						} else {
							log.Println("No message handler defined, acknowledging message without processing.")
							d.Ack(false)
						}
					}
				}
			})

			if err != nil {
				if errors.Is(err, gobreaker.ErrOpenState) {
					log.Println("Circuit breaker is OPEN for consumer. Waiting before retrying...")
					time.Sleep(c.cbTimeout)
				} else if errors.Is(err, context.Canceled) {
					log.Println("Consumer context cancelled. Exiting StartConsume.")
					return // Exit StartConsume if context was cancelled
				} else {
					log.Printf("Error during consumer circuit breaker execution: %v. Retrying consumption in 5 seconds...", err)
					time.Sleep(5 * time.Second)
				}
			} else {
				log.Println("Consumer Execute call returned nil error, but loop exited. This is unexpected. Retrying in 5 seconds...")
				time.Sleep(5 * time.Second)
			}
		}
	}
}
