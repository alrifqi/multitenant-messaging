package database

import (
	"fmt"
	"log"

	"github.com/alrifqi/multitenant-messaging/internal/config"
	"github.com/alrifqi/multitenant-messaging/internal/models"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// DB represents the database connection
type DB struct {
	*sqlx.DB
}

// NewConnection creates a new database connection
func NewConnection(cfg *config.DatabaseConfig) (*DB, error) {
	dsn := cfg.GetDSN()

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to databasex: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Database connection established successfully")
	return &DB{db}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}

// Repository interfaces
type TenantRepository interface {
	Create(tenant *models.Tenant) error
	GetByID(id int) (*models.Tenant, error)
	GetAll() ([]models.Tenant, error)
	Update(tenant *models.Tenant) error
	Delete(id int) error
}

type MessageRepository interface {
	Create(message *models.Message) error
	GetByID(id int, tenantID int) (*models.Message, error)
	GetByTenantID(tenantID int, limit, offset int) ([]models.Message, error)
	Update(message *models.Message) error
	GetPendingMessages(tenantID int, limit int) ([]models.Message, error)
	UpdateStatus(id int, tenantID int, status string, errorMessage *string) error
	IncrementRetryCount(id int, tenantID int) error
}

type ConsumerConfigRepository interface {
	Create(config *models.ConsumerConfig) error
	GetByTenantAndQueue(tenantID int, queueName string) (*models.ConsumerConfig, error)
	Update(config *models.ConsumerConfig) error
	GetAll() ([]models.ConsumerConfig, error)
	GetByTenantID(tenantID int) ([]models.ConsumerConfig, error)
}
