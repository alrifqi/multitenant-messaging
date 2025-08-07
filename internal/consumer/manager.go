package consumer

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alrifqi/multitenant-messaging/internal/config"
	"github.com/alrifqi/multitenant-messaging/internal/database"
	"github.com/alrifqi/multitenant-messaging/internal/models"
	"github.com/alrifqi/multitenant-messaging/internal/rabbitmq"
)

// Manager manages consumers for different tenants
type Manager struct {
	config             *config.ConsumerConfig
	rabbitMQ           *rabbitmq.RabbitMQ
	messageRepo        database.MessageRepository
	consumerConfigRepo database.ConsumerConfigRepository

	consumers      map[string]*Consumer
	consumersMutex sync.RWMutex

	// Metrics
	activeWorkers  int64
	totalMessages  int64
	failedMessages int64
}

// Consumer represents a consumer for a specific tenant queue
type Consumer struct {
	TenantID    int
	QueueName   string
	WorkerCount int
	IsActive    bool

	ctx          context.Context
	cancel       context.CancelFunc
	workerCount  int64
	messageCount int64
	errorCount   int64

	rabbitMQ    *rabbitmq.RabbitMQ
	messageRepo database.MessageRepository
	config      *config.ConsumerConfig
}

// NewManager creates a new consumer manager
func NewManager(
	cfg *config.ConsumerConfig,
	rabbitMQ *rabbitmq.RabbitMQ,
	messageRepo database.MessageRepository,
	consumerConfigRepo database.ConsumerConfigRepository,
) *Manager {
	return &Manager{
		config:             cfg,
		rabbitMQ:           rabbitMQ,
		messageRepo:        messageRepo,
		consumerConfigRepo: consumerConfigRepo,
		consumers:          make(map[string]*Consumer),
	}
}

// StartConsumer starts a consumer for a tenant
func (m *Manager) StartConsumer(tenantID int, queueName string, workerCount int) error {
	consumerKey := fmt.Sprintf("%d_%s", tenantID, queueName)

	m.consumersMutex.Lock()
	defer m.consumersMutex.Unlock()

	// Check if consumer already exists
	if existing, exists := m.consumers[consumerKey]; exists {
		if existing.IsActive {
			return fmt.Errorf("consumer already running for tenant %d queue %s", tenantID, queueName)
		}
		// Stop existing consumer if it's not active
		existing.Stop()
	}

	// Create new consumer
	ctx, cancel := context.WithCancel(context.Background())
	consumer := &Consumer{
		TenantID:    tenantID,
		QueueName:   queueName,
		WorkerCount: workerCount,
		IsActive:    true,
		ctx:         ctx,
		cancel:      cancel,
		rabbitMQ:    m.rabbitMQ,
		messageRepo: m.messageRepo,
		config:      m.config,
	}

	// Start consuming messages
	err := m.rabbitMQ.ConsumeMessages(ctx, queueName, workerCount, consumer.handleMessage)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to start consumer: %w", err)
	}

	m.consumers[consumerKey] = consumer
	log.Printf("Started consumer for tenant %d queue %s with %d workers", tenantID, queueName, workerCount)

	return nil
}

// StopConsumer stops a consumer for a tenant
func (m *Manager) StopConsumer(tenantID int, queueName string) error {
	consumerKey := fmt.Sprintf("%d_%s", tenantID, queueName)

	m.consumersMutex.Lock()
	defer m.consumersMutex.Unlock()

	consumer, exists := m.consumers[consumerKey]
	if !exists {
		return fmt.Errorf("consumer not found for tenant %d queue %s", tenantID, queueName)
	}

	consumer.Stop()
	delete(m.consumers, consumerKey)

	log.Printf("Stopped consumer for tenant %d queue %s", tenantID, queueName)
	return nil
}

// UpdateConsumerWorkers updates the number of workers for a consumer
func (m *Manager) UpdateConsumerWorkers(tenantID int, queueName string, workerCount int) error {
	consumerKey := fmt.Sprintf("%d_%s", tenantID, queueName)

	m.consumersMutex.Lock()
	defer m.consumersMutex.Unlock()

	consumer, exists := m.consumers[consumerKey]
	if !exists {
		return fmt.Errorf("consumer not found for tenant %d queue %s", tenantID, queueName)
	}

	// Stop current consumer
	consumer.Stop()

	// Start new consumer with updated worker count
	ctx, cancel := context.WithCancel(context.Background())
	consumer.ctx = ctx
	consumer.cancel = cancel
	consumer.WorkerCount = workerCount
	consumer.IsActive = true

	err := m.rabbitMQ.ConsumeMessages(ctx, queueName, workerCount, consumer.handleMessage)
	if err != nil {
		cancel()
		consumer.IsActive = false
		return fmt.Errorf("failed to update consumer: %w", err)
	}

	log.Printf("Updated consumer for tenant %d queue %s to %d workers", tenantID, queueName, workerCount)
	return nil
}

// GetConsumerStatus returns the status of a consumer
func (m *Manager) GetConsumerStatus(tenantID int, queueName string) (*ConsumerStatus, error) {
	consumerKey := fmt.Sprintf("%d_%s", tenantID, queueName)

	m.consumersMutex.RLock()
	defer m.consumersMutex.RUnlock()

	consumer, exists := m.consumers[consumerKey]
	if !exists {
		return nil, fmt.Errorf("consumer not found for tenant %d queue %s", tenantID, queueName)
	}

	return &ConsumerStatus{
		TenantID:      consumer.TenantID,
		QueueName:     consumer.QueueName,
		WorkerCount:   consumer.WorkerCount,
		IsActive:      consumer.IsActive,
		ActiveWorkers: atomic.LoadInt64(&consumer.workerCount),
		MessageCount:  atomic.LoadInt64(&consumer.messageCount),
		ErrorCount:    atomic.LoadInt64(&consumer.errorCount),
	}, nil
}

// GetAllConsumerStatus returns the status of all consumers
func (m *Manager) GetAllConsumerStatus() []*ConsumerStatus {
	m.consumersMutex.RLock()
	defer m.consumersMutex.RUnlock()

	var statuses []*ConsumerStatus
	for _, consumer := range m.consumers {
		status := &ConsumerStatus{
			TenantID:      consumer.TenantID,
			QueueName:     consumer.QueueName,
			WorkerCount:   consumer.WorkerCount,
			IsActive:      consumer.IsActive,
			ActiveWorkers: atomic.LoadInt64(&consumer.workerCount),
			MessageCount:  atomic.LoadInt64(&consumer.messageCount),
			ErrorCount:    atomic.LoadInt64(&consumer.errorCount),
		}
		statuses = append(statuses, status)
	}

	return statuses
}

// Stop stops all consumers
func (m *Manager) Stop() {
	m.consumersMutex.Lock()
	defer m.consumersMutex.Unlock()

	for _, consumer := range m.consumers {
		consumer.Stop()
	}

	log.Println("All consumers stopped")
}

// handleMessage processes a message for a consumer
func (c *Consumer) handleMessage(message *models.Message) error {
	atomic.AddInt64(&c.workerCount, 1)
	defer atomic.AddInt64(&c.workerCount, -1)

	atomic.AddInt64(&c.messageCount, 1)

	// Update message status to processing
	now := time.Now()
	message.Status = models.MessageStatusProcessing
	message.ProcessedAt = &now

	err := c.messageRepo.Update(message)
	if err != nil {
		atomic.AddInt64(&c.errorCount, 1)
		return fmt.Errorf("failed to update message status: %w", err)
	}

	// Simulate message processing (replace with actual business logic)
	time.Sleep(100 * time.Millisecond)

	// Update message status to completed
	message.Status = models.MessageStatusCompleted
	err = c.messageRepo.Update(message)
	if err != nil {
		atomic.AddInt64(&c.errorCount, 1)
		return fmt.Errorf("failed to update message status: %w", err)
	}

	return nil
}

// Stop stops the consumer
func (c *Consumer) Stop() {
	if c.IsActive {
		c.cancel()
		c.IsActive = false
	}
}

// ConsumerStatus represents the status of a consumer
type ConsumerStatus struct {
	TenantID      int    `json:"tenant_id"`
	QueueName     string `json:"queue_name"`
	WorkerCount   int    `json:"worker_count"`
	IsActive      bool   `json:"is_active"`
	ActiveWorkers int64  `json:"active_workers"`
	MessageCount  int64  `json:"message_count"`
	ErrorCount    int64  `json:"error_count"`
}
