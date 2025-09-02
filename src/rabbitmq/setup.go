package rabbitmq

import (
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	MQURL                = "amqp://guest:guest@rabbitmq:5672/"
	MaxConnectionRetries = 5
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func InitRabbitMQ(exchangeName, exchangeType string) (*amqp.Connection, *amqp.Channel, error) {
	var conn *amqp.Connection
	var err error

	for i := 0; i < MaxConnectionRetries; i++ {
		log.Printf("Attempting to connect to RabbitMQ at %s (Attempt %d/%d)...", MQURL, i+1, MaxConnectionRetries)
		conn, err = amqp.Dial(MQURL)
		if err == nil {
			log.Println("Successfully connected to RabbitMQ!")
			break
		}
		retryDelay := ExponentialBackoff(i)
		log.Printf("Failed to connect to RabbitMQ: %v. Retrying in %v...", err, retryDelay)
		time.Sleep(retryDelay)
	}
	failOnError(err, fmt.Sprintf("Failed to connect to RabbitMQ after %d retries", MaxConnectionRetries))

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	log.Println("Channel opened successfully.")

	err = ch.ExchangeDeclare(
		exchangeName,
		exchangeType,
		true,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, fmt.Sprintf("Failed to declare exchange '%s'", exchangeName))
	log.Printf("Exchange '%s' of type '%s' declared successfully.", exchangeName, exchangeType)

	return conn, ch, nil
}

func DeclareAndBindQueue(ch *amqp.Channel, queueName, bindingKey, exchangeName string) (amqp.Queue, error) {

	q, err := ch.QueueDeclare(
		queueName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return amqp.Queue{}, fmt.Errorf("failed to declare queue '%s': %w", queueName, err)
	}
	log.Printf("Queue '%s' declared successfully.", q.Name)

	err = ch.QueueBind(
		q.Name,
		bindingKey,
		exchangeName,
		false,
		nil,
	)
	if err != nil {
		return amqp.Queue{}, fmt.Errorf("failed to bind queue '%s' to exchange '%s' with key '%s': %w", q.Name, exchangeName, bindingKey, err)
	}
	log.Printf("Queue '%s' bound to exchange '%s' with key '%s' successfully.", q.Name, exchangeName, bindingKey)

	return q, nil
}

func CloseResources(ch *amqp.Channel, conn *amqp.Connection) func() error {
	return func() error {
		var errCh error
		var errConn error

		if ch != nil {
			log.Println("Closing RabbitMQ channel...")
			errCh = ch.Close()
		}
		if conn != nil {
			log.Println("Closing RabbitMQ connection...")
			errConn = conn.Close()
		}

		if errCh != nil {
			return fmt.Errorf("error closing channel: %w", errCh)
		}
		if errConn != nil {
			return fmt.Errorf("error closing connection: %w", errConn)
		}
		log.Println("RabbitMQ resources closed.")
		return nil
	}
}
