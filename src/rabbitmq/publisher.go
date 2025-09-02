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

type Publisher struct {
	channel  *amqp.Channel
	exchange string
	cb       *gobreaker.CircuitBreaker
}

func NewPublisher(ch *amqp.Channel, exchangeName string) *Publisher {
	settings := gobreaker.Settings{
		Name:        "RabbitMQPublisher",
		MaxRequests: 3,
		Interval:    30 * time.Second,
		Timeout:     10 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
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

func (p *Publisher) Publish(routingKey string, body []byte, headers amqp.Table) error {
	_, err := p.cb.Execute(func() (interface{}, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

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
			log.Printf("Actual publish attempt failed: %v", err)
			return nil, err
		}
		return nil, nil
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
