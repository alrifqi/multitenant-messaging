package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/alrifqi/multitenant-messaging/internal/models"
)

// tenantRepository implements TenantRepository
type tenantRepository struct {
	db *DB
}

// NewTenantRepository creates a new tenant repository
func NewTenantRepository(db *DB) TenantRepository {
	return &tenantRepository{db: db}
}

func (r *tenantRepository) Create(tenant *models.Tenant) error {
	query := `
		INSERT INTO tenants (name, created_at, updated_at)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`

	now := time.Now()
	return r.db.DB.QueryRowx(query, tenant.Name, now, now).StructScan(tenant)
}

func (r *tenantRepository) GetByID(id int) (*models.Tenant, error) {
	var tenant models.Tenant
	query := `SELECT * FROM tenants WHERE id = $1`

	err := r.db.DB.Get(&tenant, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tenant not found")
		}
		return nil, err
	}

	return &tenant, nil
}

func (r *tenantRepository) GetAll() ([]models.Tenant, error) {
	var tenants []models.Tenant
	query := `SELECT * FROM tenants ORDER BY id`

	err := r.db.DB.Select(&tenants, query)
	return tenants, err
}

func (r *tenantRepository) Update(tenant *models.Tenant) error {
	query := `
		UPDATE tenants 
		SET name = $1, updated_at = $2
		WHERE id = $3
		RETURNING updated_at
	`

	now := time.Now()
	return r.db.DB.QueryRowx(query, tenant.Name, now, tenant.ID).Scan(&tenant.UpdatedAt)
}

func (r *tenantRepository) Delete(id int) error {
	query := `DELETE FROM tenants WHERE id = $1`
	result, err := r.db.DB.Exec(query, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("tenant not found")
	}

	return nil
}

// messageRepository implements MessageRepository
type messageRepository struct {
	db *DB
}

// NewMessageRepository creates a new message repository
func NewMessageRepository(db *DB) MessageRepository {
	return &messageRepository{db: db}
}

func (r *messageRepository) Create(message *models.Message) error {
	query := `
		INSERT INTO messages (tenant_id, queue_name, message_body, status, retry_count, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`

	now := time.Now()
	return r.db.DB.QueryRowx(query,
		message.TenantID,
		message.QueueName,
		message.MessageBody,
		message.Status,
		message.RetryCount,
		now,
	).StructScan(message)
}

func (r *messageRepository) GetByID(id int, tenantID int) (*models.Message, error) {
	var message models.Message
	query := `SELECT * FROM messages WHERE id = $1 AND tenant_id = $2`

	err := r.db.DB.Get(&message, query, id, tenantID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("message not found")
		}
		return nil, err
	}

	return &message, nil
}

func (r *messageRepository) GetByTenantID(tenantID int, limit, offset int) ([]models.Message, error) {
	var messages []models.Message
	query := `
		SELECT * FROM messages 
		WHERE tenant_id = $1 
		ORDER BY created_at DESC 
		LIMIT $2 OFFSET $3
	`

	err := r.db.DB.Select(&messages, query, tenantID, limit, offset)
	return messages, err
}

func (r *messageRepository) Update(message *models.Message) error {
	query := `
		UPDATE messages 
		SET status = $1, retry_count = $2, error_message = $3, processed_at = $4
		WHERE id = $5 AND tenant_id = $6
		RETURNING processed_at
	`

	return r.db.DB.QueryRowx(query,
		message.Status,
		message.RetryCount,
		message.ErrorMessage,
		message.ProcessedAt,
		message.ID,
		message.TenantID,
	).Scan(&message.ProcessedAt)
}

func (r *messageRepository) GetPendingMessages(tenantID int, limit int) ([]models.Message, error) {
	var messages []models.Message
	query := `
		SELECT * FROM messages 
		WHERE tenant_id = $1 AND status = $2
		ORDER BY created_at ASC 
		LIMIT $3
	`

	err := r.db.DB.Select(&messages, query, tenantID, models.MessageStatusPending, limit)
	return messages, err
}

func (r *messageRepository) UpdateStatus(id int, tenantID int, status string, errorMessage *string) error {
	query := `
		UPDATE messages 
		SET status = $1, error_message = $2, processed_at = $3
		WHERE id = $4 AND tenant_id = $5
	`

	now := time.Now()
	_, err := r.db.DB.Exec(query, status, errorMessage, now, id, tenantID)
	return err
}

func (r *messageRepository) IncrementRetryCount(id int, tenantID int) error {
	query := `
		UPDATE messages 
		SET retry_count = retry_count + 1
		WHERE id = $1 AND tenant_id = $2
	`

	_, err := r.db.DB.Exec(query, id, tenantID)
	return err
}

// consumerConfigRepository implements ConsumerConfigRepository
type consumerConfigRepository struct {
	db *DB
}

// NewConsumerConfigRepository creates a new consumer config repository
func NewConsumerConfigRepository(db *DB) ConsumerConfigRepository {
	return &consumerConfigRepository{db: db}
}

func (r *consumerConfigRepository) Create(config *models.ConsumerConfig) error {
	query := `
		INSERT INTO consumer_configs (tenant_id, queue_name, workers, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`

	now := time.Now()
	return r.db.DB.QueryRowx(query,
		config.TenantID,
		config.QueueName,
		config.Workers,
		config.IsActive,
		now,
		now,
	).StructScan(config)
}

func (r *consumerConfigRepository) GetByTenantAndQueue(tenantID int, queueName string) (*models.ConsumerConfig, error) {
	var config models.ConsumerConfig
	query := `SELECT * FROM consumer_configs WHERE tenant_id = $1 AND queue_name = $2`

	err := r.db.DB.Get(&config, query, tenantID, queueName)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("consumer config not found")
		}
		return nil, err
	}

	return &config, nil
}

func (r *consumerConfigRepository) Update(config *models.ConsumerConfig) error {
	query := `
		UPDATE consumer_configs 
		SET workers = $1, is_active = $2, updated_at = $3
		WHERE id = $4
		RETURNING updated_at
	`

	now := time.Now()
	return r.db.DB.QueryRowx(query, config.Workers, config.IsActive, now, config.ID).Scan(&config.UpdatedAt)
}

func (r *consumerConfigRepository) GetAll() ([]models.ConsumerConfig, error) {
	var configs []models.ConsumerConfig
	query := `SELECT * FROM consumer_configs ORDER BY tenant_id, queue_name`

	err := r.db.DB.Select(&configs, query)
	return configs, err
}

func (r *consumerConfigRepository) GetByTenantID(tenantID int) ([]models.ConsumerConfig, error) {
	var configs []models.ConsumerConfig
	query := `SELECT * FROM consumer_configs WHERE tenant_id = $1 ORDER BY queue_name`

	err := r.db.DB.Select(&configs, query, tenantID)
	return configs, err
}
