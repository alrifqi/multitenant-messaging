package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// _ "github.com/alrifqi/multitenant-messaging/docs"
	"github.com/alrifqi/multitenant-messaging/internal/api"
	"github.com/alrifqi/multitenant-messaging/internal/auth"
	"github.com/alrifqi/multitenant-messaging/internal/config"
	"github.com/alrifqi/multitenant-messaging/internal/consumer"
	"github.com/alrifqi/multitenant-messaging/internal/database"
	"github.com/alrifqi/multitenant-messaging/internal/metrics"
	"github.com/alrifqi/multitenant-messaging/internal/middleware"
	"github.com/alrifqi/multitenant-messaging/internal/rabbitmq"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	echoSwagger "github.com/swaggo/echo-swagger"
)

// @title Multi-Tenant Messaging API
// @version 1.0
// @description A multi-tenant messaging system with RabbitMQ and PostgreSQL
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email support@swagger.io

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:8080
// @BasePath /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize database
	db, err := database.NewConnection(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Initialize RabbitMQ
	rabbitMQ, err := rabbitmq.NewConnection(&cfg.RabbitMQ)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer rabbitMQ.Close()

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

	// Middleware
	// e.Use(echoMiddleware.Recover())
	e.Use(middleware.CORS())
	e.Use(middleware.Logging())
	e.Use(middleware.JWTAuth(jwtManager))

	// Routes
	setupRoutes(e, handler, cfg)

	// Start server
	go func() {
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		log.Printf("Server starting on %s", addr)
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop consumer manager
	consumerManager.Stop()

	// Shutdown server
	if err := e.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func setupRoutes(e *echo.Echo, handler *api.Handler, cfg *config.Config) {
	// API v1 group
	v1 := e.Group("/api/v1")

	// Health check
	e.GET("/health", handler.HealthCheck)

	// Metrics endpoint
	if cfg.Metrics.Enabled {
		e.GET(cfg.Metrics.Path, echo.WrapHandler(promhttp.Handler()))
	}

	// Swagger documentation
	e.GET("/swagger/*", echoSwagger.WrapHandler)

	// Auth routes
	auth := v1.Group("/auth")
	auth.POST("/login", handler.Login)

	// Tenant routes
	tenants := v1.Group("/tenants")
	tenants.POST("", handler.CreateTenant)
	tenants.GET("", handler.GetTenants)

	// Message routes
	tenants.POST("/:tenant_id/messages", handler.CreateMessage)
	tenants.GET("/:tenant_id/messages", handler.GetMessages)

	// Consumer routes
	tenants.PUT("/:tenant_id/consumers/:queue_name/config", handler.UpdateConsumerConfig)
	tenants.GET("/:tenant_id/consumers/:queue_name/status", handler.GetConsumerStatus)

	// Global consumer status
	v1.GET("/consumers/status", handler.GetAllConsumerStatus)
}
