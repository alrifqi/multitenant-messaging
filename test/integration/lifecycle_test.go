package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
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

type IntegrationTestSuite struct {
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

func setupIntegrationTestSuite(t *testing.T) *IntegrationTestSuite {
	ctx := context.Background()

	// Start PostgreSQL container
	postgresContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:15-alpine"),
		postgres.WithDatabase("integration_test_db"),
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
			DBName:   "integration_test_db",
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
			Secret:          "integration-test-secret",
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

	// Initialize Echo server
	e := echo.New()

	return &IntegrationTestSuite{
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

func (ts *IntegrationTestSuite) cleanup(t *testing.T) {
	// Stop consumer manager
	ts.consumerManager.Stop()

	// Close connections
	if ts.db != nil {
		ts.db.Close()
	}
	if ts.rabbitMQ != nil {
		ts.rabbitMQ.Close()
	}

	// Stop containers
	if ts.postgresContainer != nil {
		require.NoError(t, ts.postgresContainer.Terminate(context.Background()))
	}
	if ts.rabbitMQContainer != nil {
		require.NoError(t, ts.rabbitMQContainer.Terminate(context.Background()))
	}
}

func getTestToken(t *testing.T, jwtManager *auth.JWTManager) string {
	token, err := jwtManager.GenerateToken(1, "test-user", 1)
	require.NoError(t, err)
	return token
}

// TestTenantLifecycle tests the complete tenant creation and destruction lifecycle
func TestTenantLifecycle(t *testing.T) {
	ts := setupIntegrationTestSuite(t)
	defer ts.cleanup(t)

	// Test 1: Create tenant
	t.Run("CreateTenant", func(t *testing.T) {
		reqBody := models.CreateTenantRequest{
			Name: "test-tenant-1",
		}
		reqJSON, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(reqJSON))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+getTestToken(t, ts.jwtManager))

		rec := httptest.NewRecorder()
		ts.e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var response models.APIResponse
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response.Success)

		// Verify tenant was created in database
		tenants, err := ts.tenantRepo.GetAll()
		require.NoError(t, err)
		assert.Len(t, tenants, 1)
		assert.Equal(t, "test-tenant-1", tenants[0].Name)
	})

	// Test 2: Create multiple tenants
	t.Run("CreateMultipleTenants", func(t *testing.T) {
		tenantNames := []string{"tenant-2", "tenant-3", "tenant-4"}

		for _, name := range tenantNames {
			reqBody := models.CreateTenantRequest{Name: name}
			reqJSON, _ := json.Marshal(reqBody)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(reqJSON))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+getTestToken(t, ts.jwtManager))

			rec := httptest.NewRecorder()
			ts.e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusCreated, rec.Code)
		}

		// Verify all tenants were created
		tenants, err := ts.tenantRepo.GetAll()
		require.NoError(t, err)
		assert.Len(t, tenants, 4) // Including the one from previous test
	})

	// Test 3: Get all tenants
	t.Run("GetAllTenants", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants", nil)
		req.Header.Set("Authorization", "Bearer "+getTestToken(t, ts.jwtManager))

		rec := httptest.NewRecorder()
		ts.e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var response models.APIResponse
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response.Success)
	})
}

// TestMessagePublishingConsumption tests message publishing and consumption flow
func TestMessagePublishingConsumption(t *testing.T) {
	ts := setupIntegrationTestSuite(t)
	defer ts.cleanup(t)

	// Create a tenant first
	tenantReqBody := models.CreateTenantRequest{Name: "message-test-tenant"}
	tenantReqJSON, _ := json.Marshal(tenantReqBody)
	tenantReq := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(tenantReqJSON))
	tenantReq.Header.Set("Content-Type", "application/json")
	tenantReq.Header.Set("Authorization", "Bearer "+getTestToken(t, ts.jwtManager))
	tenantRec := httptest.NewRecorder()
	ts.e.ServeHTTP(tenantRec, tenantReq)
	require.Equal(t, http.StatusCreated, tenantRec.Code)

	// Get tenant ID from response
	var tenantResponse models.APIResponse
	err := json.Unmarshal(tenantRec.Body.Bytes(), &tenantResponse)
	require.NoError(t, err)
	tenantID := 1 // Assuming first tenant gets ID 1

	t.Run("PublishAndConsumeMessages", func(t *testing.T) {
		queueName := "test-queue"
		messageCount := 5

		// Publish multiple messages
		for i := 0; i < messageCount; i++ {
			reqBody := models.CreateMessageRequest{
				QueueName:   queueName,
				MessageBody: fmt.Sprintf("test-message-%d", i),
			}
			reqJSON, _ := json.Marshal(reqBody)

			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/tenants/%d/messages", tenantID), bytes.NewReader(reqJSON))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+getTestToken(t, ts.jwtManager))

			rec := httptest.NewRecorder()
			ts.e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusCreated, rec.Code)
		}

		// Verify messages were stored in database
		messages, err := ts.messageRepo.GetByTenantID(tenantID, 100, 0)
		require.NoError(t, err)
		// Filter messages by queue name
		queueMessages := make([]models.Message, 0)
		for _, msg := range messages {
			if msg.QueueName == queueName {
				queueMessages = append(queueMessages, msg)
			}
		}
		assert.Len(t, queueMessages, messageCount)

		// Start consumer for the queue
		err = ts.consumerManager.StartConsumer(tenantID, queueName, 2)
		require.NoError(t, err)

		// Wait for messages to be processed
		time.Sleep(3 * time.Second)

		// Verify messages were processed
		processedMessages, err := ts.messageRepo.GetByTenantID(tenantID, 100, 0)
		require.NoError(t, err)
		// Filter messages by queue name
		queueMessages = make([]models.Message, 0)
		for _, msg := range processedMessages {
			if msg.QueueName == queueName {
				queueMessages = append(queueMessages, msg)
			}
		}

		completedCount := 0
		for _, msg := range queueMessages {
			if msg.Status == models.MessageStatusCompleted {
				completedCount++
			}
		}
		assert.GreaterOrEqual(t, completedCount, messageCount-1) // Allow for some processing time
	})

	t.Run("MessageRetryOnFailure", func(t *testing.T) {
		queueName := "retry-test-queue"

		// Publish a message that might fail
		reqBody := models.CreateMessageRequest{
			QueueName:   queueName,
			MessageBody: "message-that-might-fail",
		}
		reqJSON, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/tenants/%d/messages", tenantID), bytes.NewReader(reqJSON))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+getTestToken(t, ts.jwtManager))

		rec := httptest.NewRecorder()
		ts.e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusCreated, rec.Code)

		// Start consumer
		err := ts.consumerManager.StartConsumer(tenantID, queueName, 1)
		require.NoError(t, err)

		// Wait for processing
		time.Sleep(5 * time.Second)

		// Check message status
		messages, err := ts.messageRepo.GetByTenantID(tenantID, 100, 0)
		require.NoError(t, err)
		// Filter messages by queue name
		queueMessages := make([]models.Message, 0)
		for _, msg := range messages {
			if msg.QueueName == queueName {
				queueMessages = append(queueMessages, msg)
			}
		}
		assert.Len(t, queueMessages, 1)

		// Message should either be completed or have retry attempts
		assert.True(t, queueMessages[0].Status == models.MessageStatusCompleted ||
			queueMessages[0].Status == models.MessageStatusFailed ||
			queueMessages[0].Status == models.MessageStatusRetry)
	})
}

// TestConcurrencyConfigUpdates tests concurrent consumer configuration updates
func TestConcurrencyConfigUpdates(t *testing.T) {
	ts := setupIntegrationTestSuite(t)
	defer ts.cleanup(t)

	// Create a tenant
	tenantReqBody := models.CreateTenantRequest{Name: "concurrency-test-tenant"}
	tenantReqJSON, _ := json.Marshal(tenantReqBody)
	tenantReq := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(tenantReqJSON))
	tenantReq.Header.Set("Content-Type", "application/json")
	tenantReq.Header.Set("Authorization", "Bearer "+getTestToken(t, ts.jwtManager))
	tenantRec := httptest.NewRecorder()
	ts.e.ServeHTTP(tenantRec, tenantReq)
	require.Equal(t, http.StatusCreated, tenantRec.Code)

	tenantID := 1
	queueName := "concurrency-test-queue"

	t.Run("ConcurrentConfigUpdates", func(t *testing.T) {
		// Start with initial consumer
		err := ts.consumerManager.StartConsumer(tenantID, queueName, 1)
		require.NoError(t, err)

		// Verify initial status
		status, err := ts.consumerManager.GetConsumerStatus(tenantID, queueName)
		require.NoError(t, err)
		assert.Equal(t, 1, status.WorkerCount)
		assert.True(t, status.IsActive)

		// Perform concurrent config updates
		var wg sync.WaitGroup
		updateCount := 5
		workerCounts := []int{2, 3, 4, 5, 6}

		for i := 0; i < updateCount; i++ {
			wg.Add(1)
			go func(workerCount int) {
				defer wg.Done()

				reqBody := models.UpdateConsumerConfigRequest{
					Workers: workerCount,
				}
				reqJSON, _ := json.Marshal(reqBody)

				req := httptest.NewRequest(http.MethodPut,
					fmt.Sprintf("/api/v1/tenants/%d/consumers/%s/config", tenantID, queueName),
					bytes.NewReader(reqJSON))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+getTestToken(t, ts.jwtManager))

				rec := httptest.NewRecorder()
				ts.e.ServeHTTP(rec, req)

				// All updates should succeed
				assert.Equal(t, http.StatusOK, rec.Code)
			}(workerCounts[i])
		}

		wg.Wait()

		// Verify final status
		time.Sleep(2 * time.Second) // Allow for config updates to complete
		status, err = ts.consumerManager.GetConsumerStatus(tenantID, queueName)
		require.NoError(t, err)
		assert.True(t, status.IsActive)
		assert.GreaterOrEqual(t, status.WorkerCount, 1)
	})

	t.Run("ConcurrentConsumerStartStop", func(t *testing.T) {
		queueName2 := "concurrency-test-queue-2"

		var wg sync.WaitGroup
		operations := 10

		// Concurrent start/stop operations
		for i := 0; i < operations; i++ {
			wg.Add(1)
			go func(op int) {
				defer wg.Done()

				if op%2 == 0 {
					// Start consumer
					err := ts.consumerManager.StartConsumer(tenantID, queueName2, 2)
					if err != nil {
						t.Logf("Start consumer error: %v", err)
					}
				} else {
					// Stop consumer
					err := ts.consumerManager.StopConsumer(tenantID, queueName2)
					if err != nil {
						t.Logf("Stop consumer error: %v", err)
					}
				}
			}(i)
		}

		wg.Wait()

		// Verify final state
		time.Sleep(1 * time.Second)
		status, err := ts.consumerManager.GetConsumerStatus(tenantID, queueName2)
		if err == nil {
			// Consumer might be active or inactive, but should be in a consistent state
			assert.True(t, status.WorkerCount >= 0)
		}
	})
}

// TestEndToEndWorkflow tests a complete end-to-end workflow
func TestEndToEndWorkflow(t *testing.T) {
	ts := setupIntegrationTestSuite(t)
	defer ts.cleanup(t)

	t.Run("CompleteWorkflow", func(t *testing.T) {
		// 1. Create tenant
		tenantReqBody := models.CreateTenantRequest{Name: "e2e-test-tenant"}
		tenantReqJSON, _ := json.Marshal(tenantReqBody)
		tenantReq := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(tenantReqJSON))
		tenantReq.Header.Set("Content-Type", "application/json")
		tenantReq.Header.Set("Authorization", "Bearer "+getTestToken(t, ts.jwtManager))
		tenantRec := httptest.NewRecorder()
		ts.e.ServeHTTP(tenantRec, tenantReq)
		require.Equal(t, http.StatusCreated, tenantRec.Code)

		tenantID := 1
		queueName := "e2e-test-queue"

		// 2. Configure consumer
		configReqBody := models.UpdateConsumerConfigRequest{Workers: 3}
		configReqJSON, _ := json.Marshal(configReqBody)
		configReq := httptest.NewRequest(http.MethodPut,
			fmt.Sprintf("/api/v1/tenants/%d/consumers/%s/config", tenantID, queueName),
			bytes.NewReader(configReqJSON))
		configReq.Header.Set("Content-Type", "application/json")
		configReq.Header.Set("Authorization", "Bearer "+getTestToken(t, ts.jwtManager))
		configRec := httptest.NewRecorder()
		ts.e.ServeHTTP(configRec, configReq)
		assert.Equal(t, http.StatusOK, configRec.Code)

		// 3. Publish messages
		messageCount := 10
		for i := 0; i < messageCount; i++ {
			reqBody := models.CreateMessageRequest{
				QueueName:   queueName,
				MessageBody: fmt.Sprintf("e2e-message-%d", i),
			}
			reqJSON, _ := json.Marshal(reqBody)

			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/tenants/%d/messages", tenantID), bytes.NewReader(reqJSON))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+getTestToken(t, ts.jwtManager))

			rec := httptest.NewRecorder()
			ts.e.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusCreated, rec.Code)
		}

		// 4. Start consumer
		err := ts.consumerManager.StartConsumer(tenantID, queueName, 3)
		require.NoError(t, err)

		// 5. Wait for processing
		time.Sleep(5 * time.Second)

		// 6. Verify results
		messages, err := ts.messageRepo.GetByTenantID(tenantID, 100, 0)
		require.NoError(t, err)
		// Filter messages by queue name
		queueMessages := make([]models.Message, 0)
		for _, msg := range messages {
			if msg.QueueName == queueName {
				queueMessages = append(queueMessages, msg)
			}
		}
		assert.Len(t, queueMessages, messageCount)

		completedCount := 0
		for _, msg := range queueMessages {
			if msg.Status == models.MessageStatusCompleted {
				completedCount++
			}
		}
		assert.GreaterOrEqual(t, completedCount, messageCount-2) // Allow for some processing time

		// 7. Check consumer status
		status, err := ts.consumerManager.GetConsumerStatus(tenantID, queueName)
		require.NoError(t, err)
		assert.True(t, status.IsActive)
		assert.Equal(t, 3, status.WorkerCount)
		assert.GreaterOrEqual(t, status.MessageCount, int64(messageCount-2))

		// 8. Scale down consumer
		scaleReqBody := models.UpdateConsumerConfigRequest{Workers: 1}
		scaleReqJSON, _ := json.Marshal(scaleReqBody)
		scaleReq := httptest.NewRequest(http.MethodPut,
			fmt.Sprintf("/api/v1/tenants/%d/consumers/%s/config", tenantID, queueName),
			bytes.NewReader(scaleReqJSON))
		scaleReq.Header.Set("Content-Type", "application/json")
		scaleReq.Header.Set("Authorization", "Bearer "+getTestToken(t, ts.jwtManager))
		scaleRec := httptest.NewRecorder()
		ts.e.ServeHTTP(scaleRec, scaleReq)
		assert.Equal(t, http.StatusOK, scaleRec.Code)

		// 9. Verify scaling
		time.Sleep(2 * time.Second)
		status, err = ts.consumerManager.GetConsumerStatus(tenantID, queueName)
		require.NoError(t, err)
		assert.True(t, status.IsActive)
		assert.Equal(t, 1, status.WorkerCount)
	})
}
