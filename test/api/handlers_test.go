package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alrifqi/multitenant-messaging/internal/api"
	"github.com/alrifqi/multitenant-messaging/internal/auth"
	"github.com/alrifqi/multitenant-messaging/internal/config"
	"github.com/alrifqi/multitenant-messaging/internal/consumer"
	"github.com/alrifqi/multitenant-messaging/internal/database"
	"github.com/alrifqi/multitenant-messaging/internal/metrics"
	"github.com/alrifqi/multitenant-messaging/internal/models"
	"github.com/alrifqi/multitenant-messaging/internal/rabbitmq"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcrabbitmq "github.com/testcontainers/testcontainers-go/modules/rabbitmq"
	"github.com/testcontainers/testcontainers-go/wait"
)

type TestSuite struct {
	e                  *echo.Echo
	handler            *api.Handler
	tenantRepo         database.TenantRepository
	messageRepo        database.MessageRepository
	consumerConfigRepo database.ConsumerConfigRepository
	rabbitMQ           *rabbitmq.RabbitMQ
	jwtManager         *auth.JWTManager
	metrics            *metrics.Metrics
	consumerManager    *consumer.Manager
	db                 *database.DB
	postgresContainer  testcontainers.Container
	rabbitMQContainer  testcontainers.Container
}

func setupTestSuite(t *testing.T) *TestSuite {
	ctx := context.Background()

	// Start PostgreSQL container
	postgresContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:15-alpine"),
		postgres.WithDatabase("test_db"),
		postgres.WithUsername("test_user"),
		postgres.WithPassword("test_password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections"),
		),
	)
	require.NoError(t, err)

	// Start RabbitMQ container
	rabbitMQContainer, err := tcrabbitmq.RunContainer(ctx,
		testcontainers.WithImage("rabbitmq:3-management-alpine"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("Server startup complete"),
		),
	)
	require.NoError(t, err)

	// Get container endpoints
	postgresHost, err := postgresContainer.Host(ctx)
	require.NoError(t, err)
	postgresPort, err := postgresContainer.MappedPort(ctx, "5432")
	require.NoError(t, err)

	rabbitMQHost, err := rabbitMQContainer.Host(ctx)
	require.NoError(t, err)
	rabbitMQPort, err := rabbitMQContainer.MappedPort(ctx, "5672")
	require.NoError(t, err)

	// Create test configuration
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Host:     postgresHost,
			Port:     postgresPort.Int(),
			User:     "test_user",
			Password: "test_password",
			DBName:   "test_db",
			SSLMode:  "disable",
		},
		RabbitMQ: config.RabbitMQConfig{
			Host:     rabbitMQHost,
			Port:     rabbitMQPort.Int(),
			User:     "guest",
			Password: "guest",
			VHost:    "/",
		},
		JWT: config.JWTConfig{
			Secret:          "test-secret",
			ExpirationHours: 24,
		},
		Consumer: config.ConsumerConfig{
			DefaultWorkers:   3,
			MaxWorkers:       10,
			WorkerBufferSize: 100,
			RetryAttempts:    3,
			RetryDelay:       5 * time.Second,
		},
	}

	// Initialize database
	db, err := database.NewConnection(&cfg.Database)
	require.NoError(t, err)

	// Run migrations (you might need to implement this)
	// runMigrations(t, db)

	// Initialize RabbitMQ
	rabbitMQ, err := rabbitmq.NewConnection(&cfg.RabbitMQ)
	require.NoError(t, err)

	// Initialize repositories
	tenantRepo := database.NewTenantRepository(db)
	messageRepo := database.NewMessageRepository(db)
	consumerConfigRepo := database.NewConsumerConfigRepository(db)

	// Initialize JWT manager
	jwtManager := auth.NewJWTManager(&cfg.JWT)

	// Initialize metrics
	metrics := metrics.NewMetrics()

	// Initialize consumer manager
	consumerManager := consumer.NewManager(
		&cfg.Consumer,
		rabbitMQ,
		messageRepo,
		consumerConfigRepo,
	)

	// Initialize API handler
	handler := api.NewHandler(
		tenantRepo,
		messageRepo,
		consumerConfigRepo,
		consumerManager,
		rabbitMQ,
		jwtManager,
		metrics,
	)

	// Initialize Echo
	e := echo.New()

	return &TestSuite{
		e:                  e,
		handler:            handler,
		tenantRepo:         tenantRepo,
		messageRepo:        messageRepo,
		consumerConfigRepo: consumerConfigRepo,
		rabbitMQ:           rabbitMQ,
		jwtManager:         jwtManager,
		metrics:            metrics,
		consumerManager:    consumerManager,
		db:                 db,
		postgresContainer:  postgresContainer,
		rabbitMQContainer:  rabbitMQContainer,
	}
}

func (ts *TestSuite) cleanup(t *testing.T) {
	ctx := context.Background()

	ts.consumerManager.Stop()
	ts.rabbitMQ.Close()
	ts.db.Close()

	if ts.postgresContainer != nil {
		ts.postgresContainer.Terminate(ctx)
	}
	if ts.rabbitMQContainer != nil {
		ts.rabbitMQContainer.Terminate(ctx)
	}
}

func TestLogin(t *testing.T) {
	ts := setupTestSuite(t)
	defer ts.cleanup(t)

	tests := []struct {
		name           string
		request        models.LoginRequest
		expectedStatus int
		expectToken    bool
	}{
		{
			name: "Valid credentials",
			request: models.LoginRequest{
				Username: "admin",
				Password: "password",
			},
			expectedStatus: http.StatusOK,
			expectToken:    true,
		},
		{
			name: "Invalid credentials",
			request: models.LoginRequest{
				Username: "admin",
				Password: "wrongpassword",
			},
			expectedStatus: http.StatusUnauthorized,
			expectToken:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := ts.e.NewContext(req, rec)

			err := ts.handler.Login(c)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, rec.Code)

			if tt.expectToken {
				var response models.LoginResponse
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.NotEmpty(t, response.Token)
			}
		})
	}
}

func TestCreateTenant(t *testing.T) {
	ts := setupTestSuite(t)
	defer ts.cleanup(t)

	// Get JWT token first
	token := getTestToken(t, ts.jwtManager)

	tests := []struct {
		name           string
		request        models.CreateTenantRequest
		expectedStatus int
	}{
		{
			name: "Valid tenant",
			request: models.CreateTenantRequest{
				Name: "Test Tenant",
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "Empty name",
			request: models.CreateTenantRequest{
				Name: "",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/tenants", bytes.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			req.Header.Set(echo.HeaderAuthorization, fmt.Sprintf("Bearer %s", token))
			rec := httptest.NewRecorder()
			c := ts.e.NewContext(req, rec)

			err := ts.handler.CreateTenant(c)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, rec.Code)

			if tt.expectedStatus == http.StatusCreated {
				var response models.APIResponse
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.True(t, response.Success)
			}
		})
	}
}

func TestCreateMessage(t *testing.T) {
	ts := setupTestSuite(t)
	defer ts.cleanup(t)

	// Create a tenant first
	tenant := &models.Tenant{Name: "Test Tenant"}
	err := ts.tenantRepo.Create(tenant)
	require.NoError(t, err)

	token := getTestToken(t, ts.jwtManager)

	tests := []struct {
		name           string
		tenantID       int
		request        models.CreateMessageRequest
		expectedStatus int
	}{
		{
			name:     "Valid message",
			tenantID: tenant.ID,
			request: models.CreateMessageRequest{
				QueueName:   "test_queue",
				MessageBody: "Test message",
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:     "Empty queue name",
			tenantID: tenant.ID,
			request: models.CreateMessageRequest{
				QueueName:   "",
				MessageBody: "Test message",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			url := fmt.Sprintf("/tenants/%d/messages", tt.tenantID)
			req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			req.Header.Set(echo.HeaderAuthorization, fmt.Sprintf("Bearer %s", token))
			rec := httptest.NewRecorder()
			c := ts.e.NewContext(req, rec)
			c.SetParamNames("tenant_id")
			c.SetParamValues(fmt.Sprintf("%d", tt.tenantID))

			err := ts.handler.CreateMessage(c)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, rec.Code)

			if tt.expectedStatus == http.StatusCreated {
				var response models.APIResponse
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.True(t, response.Success)
			}
		})
	}
}

func TestUpdateConsumerConfig(t *testing.T) {
	ts := setupTestSuite(t)
	defer ts.cleanup(t)

	// Create a tenant first
	tenant := &models.Tenant{Name: "Test Tenant"}
	err := ts.tenantRepo.Create(tenant)
	require.NoError(t, err)

	token := getTestToken(t, ts.jwtManager)

	tests := []struct {
		name           string
		tenantID       int
		queueName      string
		request        models.UpdateConsumerConfigRequest
		expectedStatus int
	}{
		{
			name:      "Valid config",
			tenantID:  tenant.ID,
			queueName: "test_queue",
			request: models.UpdateConsumerConfigRequest{
				Workers: 5,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:      "Invalid worker count",
			tenantID:  tenant.ID,
			queueName: "test_queue",
			request: models.UpdateConsumerConfigRequest{
				Workers: 15, // Exceeds max
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			url := fmt.Sprintf("/tenants/%d/consumers/%s/config", tt.tenantID, tt.queueName)
			req := httptest.NewRequest(http.MethodPut, url, bytes.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			req.Header.Set(echo.HeaderAuthorization, fmt.Sprintf("Bearer %s", token))
			rec := httptest.NewRecorder()
			c := ts.e.NewContext(req, rec)
			c.SetParamNames("tenant_id", "queue_name")
			c.SetParamValues(fmt.Sprintf("%d", tt.tenantID), tt.queueName)

			err := ts.handler.UpdateConsumerConfig(c)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, rec.Code)

			if tt.expectedStatus == http.StatusOK {
				var response models.APIResponse
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.True(t, response.Success)
			}
		})
	}
}

func TestHealthCheck(t *testing.T) {
	ts := setupTestSuite(t)
	defer ts.cleanup(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	c := ts.e.NewContext(req, rec)

	err := ts.handler.HealthCheck(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var response models.APIResponse
	err = json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response.Success)
	assert.Equal(t, "Service is healthy", response.Message)
}

func getTestToken(t *testing.T, jwtManager *auth.JWTManager) string {
	token, err := jwtManager.GenerateToken(1, "admin", 1)
	require.NoError(t, err)
	return token
}
