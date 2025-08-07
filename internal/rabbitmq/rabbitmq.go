package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/alrifqi/multitenant-messaging/internal/config"
	"github.com/alrifqi/multitenant-messaging/internal/models"

	"github.com/streadway/amqp"
)

// RabbitMQ represents the RabbitMQ connection and channel
type RabbitMQ struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	config  *config.RabbitMQConfig
}

// NewConnection creates a new RabbitMQ connection
func NewConnection(cfg *config.RabbitMQConfig) (*RabbitMQ, error) {
	url := cfg.GetURL()

	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	rabbitMQ := &RabbitMQ{
		conn:    conn,
		channel: ch,
		config:  cfg,
	}

	log.Println("RabbitMQ connection established successfully")
	return rabbitMQ, nil
}

// Close closes the RabbitMQ connection
func (r *RabbitMQ) Close() error {
	if r.channel != nil {
		r.channel.Close()
	}
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}

// DeclareQueue declares a queue for a tenant
func (r *RabbitMQ) DeclareQueue(queueName string) error {
	_, err := r.channel.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	return err
}

// DeclareDLQ declares a dead letter queue for failed messages
func (r *RabbitMQ) DeclareDLQ(queueName string) error {
	dlqName := fmt.Sprintf("%s%s", models.DLQPrefix, queueName)

	_, err := r.channel.QueueDeclare(
		dlqName, // name
		true,    // durable
		false,   // delete when unused
		false,   // exclusive
		false,   // no-wait
		nil,     // arguments
	)
	return err
}

// PublishMessage publishes a message to a queue
func (r *RabbitMQ) PublishMessage(queueName string, message *models.Message) error {
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	err = r.channel.Publish(
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
		},
	)
	return err
}

// ConsumeMessages starts consuming messages from a queue
func (r *RabbitMQ) ConsumeMessages(ctx context.Context, queueName string, workerCount int, messageHandler func(*models.Message) error) error {
	// Declare the main queue
	if err := r.DeclareQueue(queueName); err != nil {
		return fmt.Errorf("failed to declare queue %s: %w", queueName, err)
	}

	// Declare the dead letter queue
	if err := r.DeclareDLQ(queueName); err != nil {
		return fmt.Errorf("failed to declare DLQ for %s: %w", queueName, err)
	}

	// Set QoS for fair dispatch
	err := r.channel.Qos(
		workerCount, // prefetch count
		0,           // prefetch size
		false,       // global
	)
	if err != nil {
		return fmt.Errorf("failed to set QoS: %w", err)
	}

	msgs, err := r.channel.Consume(
		queueName, // queue
		"",        // consumer
		false,     // auto-ack
		false,     // exclusive
		false,     // no-local
		false,     // no-wait
		nil,       // args
	)
	if err != nil {
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	// Start workers
	for i := 0; i < workerCount; i++ {
		go r.worker(ctx, msgs, messageHandler, queueName)
	}

	log.Printf("Started %d workers for queue: %s", workerCount, queueName)
	return nil
}

// worker processes messages from the queue
func (r *RabbitMQ) worker(ctx context.Context, msgs <-chan amqp.Delivery, messageHandler func(*models.Message) error, queueName string) {
	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker stopped for queue: %s", queueName)
			return
		case msg := <-msgs:
			r.processMessage(msg, messageHandler, queueName)
		}
	}
}

// processMessage processes a single message
func (r *RabbitMQ) processMessage(msg amqp.Delivery, messageHandler func(*models.Message) error, queueName string) {
	var message models.Message
	if err := json.Unmarshal(msg.Body, &message); err != nil {
		log.Printf("Failed to unmarshal message: %v", err)
		msg.Nack(false, false) // Reject without requeue
		return
	}

	// Process the message
	err := messageHandler(&message)
	if err != nil {
		log.Printf("Failed to process message %d: %v", message.ID, err)

		// Check if we should retry or send to DLQ
		if message.RetryCount < 3 {
			// Requeue for retry
			msg.Nack(false, true)
		} else {
			// Send to dead letter queue
			dlqName := fmt.Sprintf("%s%s", models.DLQPrefix, queueName)
			r.sendToDLQ(dlqName, &message)
			msg.Ack(false)
		}
		return
	}

	// Acknowledge successful processing
	msg.Ack(false)
}

// sendToDLQ sends a failed message to the dead letter queue
func (r *RabbitMQ) sendToDLQ(dlqName string, message *models.Message) error {
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message for DLQ: %w", err)
	}

	err = r.channel.Publish(
		"",      // exchange
		dlqName, // routing key
		false,   // mandatory
		false,   // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
		},
	)
	return err
}

// GetQueueDepth returns the number of messages in a queue
func (r *RabbitMQ) GetQueueDepth(queueName string) (int, error) {
	queue, err := r.channel.QueueInspect(queueName)
	if err != nil {
		return 0, fmt.Errorf("failed to inspect queue %s: %w", queueName, err)
	}
	return queue.Messages, nil
}
