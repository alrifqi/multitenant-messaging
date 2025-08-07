package models

import (
	"time"
)

// Tenant represents a tenant in the system
type Tenant struct {
	ID        int       `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// Message represents a message in the system
type Message struct {
	ID           int        `db:"id" json:"id"`
	TenantID     int        `db:"tenant_id" json:"tenant_id"`
	QueueName    string     `db:"queue_name" json:"queue_name"`
	MessageBody  string     `db:"message_body" json:"message_body"`
	Status       string     `db:"status" json:"status"`
	RetryCount   int        `db:"retry_count" json:"retry_count"`
	ErrorMessage *string    `db:"error_message" json:"error_message,omitempty"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	ProcessedAt  *time.Time `db:"processed_at" json:"processed_at,omitempty"`
}

// ConsumerConfig represents consumer configuration for a tenant
type ConsumerConfig struct {
	ID        int       `db:"id" json:"id"`
	TenantID  int       `db:"tenant_id" json:"tenant_id"`
	QueueName string    `db:"queue_name" json:"queue_name"`
	Workers   int       `db:"workers" json:"workers"`
	IsActive  bool      `db:"is_active" json:"is_active"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// CreateTenantRequest represents a request to create a tenant
type CreateTenantRequest struct {
	Name string `json:"name" validate:"required"`
}

// CreateMessageRequest represents a request to create a message
type CreateMessageRequest struct {
	QueueName   string `json:"queue_name" validate:"required"`
	MessageBody string `json:"message_body" validate:"required"`
}

// UpdateConsumerConfigRequest represents a request to update consumer configuration
type UpdateConsumerConfigRequest struct {
	Workers int `json:"workers" validate:"required,min=1,max=10"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Token string `json:"token"`
}

// APIResponse represents a generic API response
type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// MessageStatus constants
const (
	MessageStatusPending    = "pending"
	MessageStatusProcessing = "processing"
	MessageStatusCompleted  = "completed"
	MessageStatusFailed     = "failed"
	MessageStatusRetry      = "retry"
)

// Queue constants
const (
	DefaultQueueName = "tenant_1_queue"
	DLQPrefix        = "dlq_"
)
