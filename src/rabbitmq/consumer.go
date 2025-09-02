package rabbitmq

import (
	"context"
	"errors"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sony/gobreaker"
	"log"
	"time"
)

type MessageHandler func(delivery amqp.Delivery) bool

type Consumer struct {
	channel        *amqp.Channel
	queueName      string
	messageHandler MessageHandler
	cb             *gobreaker.CircuitBreaker
	cbTimeout      time.Duration
}

func NewConsumer(ch *amqp.Channel, queueName string, handler MessageHandler) *Consumer {
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

func (c *Consumer) StartConsume(ctx context.Context) {
	log.Printf("Consumer starting to listen on queue: %s", c.queueName)

	for {
		select {
		case <-ctx.Done():
			log.Println("Consumer received shutdown signal. Exiting consume loop.")
			return

		default:
			_, err := c.cb.Execute(func() (interface{}, error) {
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

				for {
					select {
					case <-ctx.Done():
						log.Println("Context cancelled during message processing. Stopping inner consume loop.")
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
