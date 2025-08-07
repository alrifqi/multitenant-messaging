package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/alrifqi/multitenant-messaging/internal/auth"
	"github.com/alrifqi/multitenant-messaging/internal/consumer"
	"github.com/alrifqi/multitenant-messaging/internal/database"
	"github.com/alrifqi/multitenant-messaging/internal/metrics"
	"github.com/alrifqi/multitenant-messaging/internal/models"
	"github.com/alrifqi/multitenant-messaging/internal/rabbitmq"

	"github.com/labstack/echo/v4"
)

// Handler represents API handlers
type Handler struct {
	tenantRepo         database.TenantRepository
	messageRepo        database.MessageRepository
	consumerConfigRepo database.ConsumerConfigRepository
	consumerManager    *consumer.Manager
	rabbitMQ           *rabbitmq.RabbitMQ
	jwtManager         *auth.JWTManager
	metrics            *metrics.Metrics
}

// NewHandler creates a new API handler
func NewHandler(
	tenantRepo database.TenantRepository,
	messageRepo database.MessageRepository,
	consumerConfigRepo database.ConsumerConfigRepository,
	consumerManager *consumer.Manager,
	rabbitMQ *rabbitmq.RabbitMQ,
	jwtManager *auth.JWTManager,
	metrics *metrics.Metrics,
) *Handler {
	return &Handler{
		tenantRepo:         tenantRepo,
		messageRepo:        messageRepo,
		consumerConfigRepo: consumerConfigRepo,
		consumerManager:    consumerManager,
		rabbitMQ:           rabbitMQ,
		jwtManager:         jwtManager,
		metrics:            metrics,
	}
}

// Login handles user authentication
// @Summary Login user
// @Description Authenticate user and return JWT token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body models.LoginRequest true "Login credentials"
// @Success 200 {object} models.LoginResponse
// @Failure 400 {object} models.APIResponse
// @Failure 401 {object} models.APIResponse
// @Router /auth/login [post]
func (h *Handler) Login(c echo.Context) error {
	var req models.LoginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid request body",
		})
	}

	// Simple authentication (replace with proper user authentication)
	if req.Username == "admin" && req.Password == "password" {
		token, err := h.jwtManager.GenerateToken(1, req.Username, 1)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.APIResponse{
				Success: false,
				Error:   "Failed to generate token",
			})
		}

		return c.JSON(http.StatusOK, models.LoginResponse{
			Token: token,
		})
	}

	return c.JSON(http.StatusUnauthorized, models.APIResponse{
		Success: false,
		Error:   "Invalid credentials",
	})
}

// CreateTenant creates a new tenant
// @Summary Create tenant
// @Description Create a new tenant
// @Tags tenants
// @Accept json
// @Produce json
// @Param request body models.CreateTenantRequest true "Tenant information"
// @Success 201 {object} models.APIResponse
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Security BearerAuth
// @Router /tenants [post]
func (h *Handler) CreateTenant(c echo.Context) error {
	var req models.CreateTenantRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid request body",
		})
	}

	tenant := &models.Tenant{
		Name: req.Name,
	}

	if err := h.tenantRepo.Create(tenant); err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to create tenant",
		})
	}

	return c.JSON(http.StatusCreated, models.APIResponse{
		Success: true,
		Message: "Tenant created successfully",
		Data:    tenant,
	})
}

// GetTenants retrieves all tenants
// @Summary Get tenants
// @Description Get all tenants
// @Tags tenants
// @Produce json
// @Success 200 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Security BearerAuth
// @Router /tenants [get]
func (h *Handler) GetTenants(c echo.Context) error {
	tenants, err := h.tenantRepo.GetAll()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to retrieve tenants",
		})
	}

	return c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    tenants,
	})
}

// CreateMessage creates a new message
// @Summary Create message
// @Description Create a new message for a tenant
// @Tags messages
// @Accept json
// @Produce json
// @Param tenant_id path int true "Tenant ID"
// @Param request body models.CreateMessageRequest true "Message information"
// @Success 201 {object} models.APIResponse
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Security BearerAuth
// @Router /tenants/{tenant_id}/messages [post]
func (h *Handler) CreateMessage(c echo.Context) error {
	tenantID, err := strconv.Atoi(c.Param("tenant_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid tenant ID",
		})
	}

	var req models.CreateMessageRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid request body",
		})
	}

	message := &models.Message{
		TenantID:    tenantID,
		QueueName:   req.QueueName,
		MessageBody: req.MessageBody,
		Status:      models.MessageStatusPending,
	}

	if err := h.messageRepo.Create(message); err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to create message",
		})
	}

	// Publish message to RabbitMQ
	if err := h.rabbitMQ.PublishMessage(req.QueueName, message); err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to publish message to queue",
		})
	}

	// Update metrics
	h.metrics.IncrementMessageTotal(tenantID, req.QueueName, models.MessageStatusPending)

	return c.JSON(http.StatusCreated, models.APIResponse{
		Success: true,
		Message: "Message created and published successfully",
		Data:    message,
	})
}

// GetMessages retrieves messages for a tenant
// @Summary Get messages
// @Description Get messages for a tenant
// @Tags messages
// @Produce json
// @Param tenant_id path int true "Tenant ID"
// @Param limit query int false "Limit" default(10)
// @Param offset query int false "Offset" default(0)
// @Success 200 {object} models.APIResponse
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Security BearerAuth
// @Router /tenants/{tenant_id}/messages [get]
func (h *Handler) GetMessages(c echo.Context) error {
	tenantID, err := strconv.Atoi(c.Param("tenant_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid tenant ID",
		})
	}

	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 10
	}

	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if offset < 0 {
		offset = 0
	}

	messages, err := h.messageRepo.GetByTenantID(tenantID, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to retrieve messages",
		})
	}

	return c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    messages,
	})
}

// UpdateConsumerConfig updates consumer configuration
// @Summary Update consumer config
// @Description Update consumer configuration for a tenant
// @Tags consumers
// @Accept json
// @Produce json
// @Param tenant_id path int true "Tenant ID"
// @Param queue_name path string true "Queue name"
// @Param request body models.UpdateConsumerConfigRequest true "Consumer configuration"
// @Success 200 {object} models.APIResponse
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Security BearerAuth
// @Router /tenants/{tenant_id}/consumers/{queue_name}/config [put]
func (h *Handler) UpdateConsumerConfig(c echo.Context) error {
	tenantID, err := strconv.Atoi(c.Param("tenant_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid tenant ID",
		})
	}

	queueName := c.Param("queue_name")
	if queueName == "" {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid queue name",
		})
	}

	var req models.UpdateConsumerConfigRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid request body",
		})
	}

	// Get or create consumer config
	config, err := h.consumerConfigRepo.GetByTenantAndQueue(tenantID, queueName)
	if err != nil {
		// Create new config if it doesn't exist
		config = &models.ConsumerConfig{
			TenantID:  tenantID,
			QueueName: queueName,
			Workers:   req.Workers,
			IsActive:  true,
		}
		if err := h.consumerConfigRepo.Create(config); err != nil {
			return c.JSON(http.StatusInternalServerError, models.APIResponse{
				Success: false,
				Error:   "Failed to create consumer config",
			})
		}
	} else {
		// Update existing config
		config.Workers = req.Workers
		if err := h.consumerConfigRepo.Update(config); err != nil {
			return c.JSON(http.StatusInternalServerError, models.APIResponse{
				Success: false,
				Error:   "Failed to update consumer config",
			})
		}
	}

	// Update consumer manager
	if err := h.consumerManager.UpdateConsumerWorkers(tenantID, queueName, req.Workers); err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to update consumer workers",
		})
	}

	// Update metrics
	h.metrics.SetActiveWorkers(tenantID, queueName, req.Workers)

	return c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Consumer configuration updated successfully",
		Data:    config,
	})
}

// GetConsumerStatus gets consumer status
// @Summary Get consumer status
// @Description Get consumer status for a tenant
// @Tags consumers
// @Produce json
// @Param tenant_id path int true "Tenant ID"
// @Param queue_name path string true "Queue name"
// @Success 200 {object} models.APIResponse
// @Failure 400 {object} models.APIResponse
// @Failure 500 {object} models.APIResponse
// @Security BearerAuth
// @Router /tenants/{tenant_id}/consumers/{queue_name}/status [get]
func (h *Handler) GetConsumerStatus(c echo.Context) error {
	tenantID, err := strconv.Atoi(c.Param("tenant_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid tenant ID",
		})
	}

	queueName := c.Param("queue_name")
	if queueName == "" {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid queue name",
		})
	}

	status, err := h.consumerManager.GetConsumerStatus(tenantID, queueName)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.APIResponse{
			Success: false,
			Error:   "Consumer not found",
		})
	}

	return c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    status,
	})
}

// GetAllConsumerStatus gets all consumer statuses
// @Summary Get all consumer statuses
// @Description Get status of all consumers
// @Tags consumers
// @Produce json
// @Success 200 {object} models.APIResponse
// @Security BearerAuth
// @Router /consumers/status [get]
func (h *Handler) GetAllConsumerStatus(c echo.Context) error {
	statuses := h.consumerManager.GetAllConsumerStatus()

	return c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    statuses,
	})
}

// HealthCheck provides health check endpoint
// @Summary Health check
// @Description Check application health
// @Tags health
// @Produce json
// @Success 200 {object} models.APIResponse
// @Router /health [get]
func (h *Handler) HealthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Service is healthy",
		Data: map[string]interface{}{
			"timestamp": time.Now().Unix(),
			"status":    "ok",
		},
	})
}
