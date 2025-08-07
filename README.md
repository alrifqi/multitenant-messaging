# Multi-Tenant Messaging System

A scalable, multi-tenant messaging system built with Go, PostgreSQL, and RabbitMQ. This system provides isolated message queues for different tenants with configurable consumers, monitoring, and authentication.

## 🏗️ Architecture Overview

The system follows a clean architecture pattern with clear separation of concerns:

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   API Layer     │    │  Consumer Layer │    │  Database Layer │
│   (Echo HTTP)   │    │   (RabbitMQ)    │    │   (PostgreSQL)  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                    ┌─────────────────┐
                    │  Monitoring     │
                    │ (Prometheus)    │
                    └─────────────────┘
```

## 📁 Project Structure

```
multitenant-messaging/
├── cmd/
│   └── server/
│       └── main.go                 # Application entry point
├── configs/
│   ├── config.yaml                 # Configuration file
│   └── migrations/                 # Database migrations
├── internal/
│   ├── api/
│   │   └── handlers.go             # HTTP request handlers
│   ├── auth/
│   │   └── jwt.go                  # JWT authentication
│   ├── config/
│   │   └── config.go               # Configuration management
│   ├── consumer/
│   │   └── manager.go              # Message consumer management
│   ├── database/
│   │   ├── database.go             # Database connection
│   │   └── repositories.go         # Data access layer
│   ├── metrics/
│   │   └── metrics.go              # Prometheus metrics
│   ├── middleware/
│   │   └── middleware.go           # HTTP middleware
│   ├── models/
│   │   └── models.go               # Data models
│   └── rabbitmq/
│       └── rabbitmq.go             # RabbitMQ client
├── monitoring/                     # Monitoring configuration
├── docker-compose.yml              # Development environment
└── README.md
```

## 🔧 Core Components

### 1. **API Layer** (`internal/api/`)
- **Handlers**: RESTful API endpoints for tenant management, message operations, and consumer configuration
- **Authentication**: JWT-based authentication for secure API access
- **Middleware**: CORS, logging, and authentication middleware

### 2. **Consumer Management** (`internal/consumer/`)
- **Manager**: Orchestrates message consumers for different tenants
- **Dynamic Scaling**: Adjusts worker count per tenant queue
- **Isolation**: Each tenant has isolated message processing
- **Retry Logic**: Handles failed messages with configurable retry attempts

### 3. **Database Layer** (`internal/database/`)
- **Repositories**: Clean data access layer for tenants, messages, and consumer configs
- **Migrations**: Version-controlled database schema changes
- **Connection Pooling**: Optimized database connections

### 4. **Message Queue** (`internal/rabbitmq/`)
- **Connection Management**: Robust RabbitMQ connection handling
- **Queue Isolation**: Separate queues per tenant
- **Dead Letter Queues**: Failed message handling

### 5. **Monitoring** (`internal/metrics/`)
- **Prometheus Integration**: Custom metrics for message processing
- **Grafana Dashboards**: Real-time monitoring and alerting
- **Health Checks**: System health monitoring

## 🚀 Key Features

### Multi-Tenancy
- **Tenant Isolation**: Each tenant has separate message queues and configurations
- **Resource Management**: Configurable worker counts per tenant
- **Data Segregation**: Database-level tenant separation

### Message Processing
- **Asynchronous Processing**: Non-blocking message consumption
- **Retry Mechanism**: Automatic retry for failed messages
- **Status Tracking**: Real-time message status monitoring
- **Error Handling**: Comprehensive error logging and dead letter queues

### Scalability
- **Dynamic Scaling**: Adjust consumer workers per tenant
- **Horizontal Scaling**: Stateless design for multiple instances
- **Connection Pooling**: Optimized database and RabbitMQ connections

### Monitoring & Observability
- **Metrics Collection**: Prometheus metrics for all operations
- **Health Monitoring**: System health endpoints
- **Logging**: Structured logging with configurable levels
- **Dashboard**: Grafana dashboards for visualization

## 🔄 System Flow

### 1. **Tenant Creation**
```
POST /api/v1/tenants
↓
Create tenant record in database
↓
Initialize default consumer configuration
```

### 2. **Message Publishing**
```
POST /api/v1/tenants/{tenant_id}/messages
↓
Validate tenant and message
↓
Store message in database
↓
Publish to tenant-specific RabbitMQ queue
```

### 3. **Message Consumption**
```
Consumer Manager
↓
Start workers for tenant queue
↓
Consume messages from RabbitMQ
↓
Process message (business logic)
↓
Update message status in database
↓
Handle failures with retry logic
```

### 4. **Consumer Management**
```
PUT /api/v1/tenants/{tenant_id}/consumers/{queue_name}/config
↓
Update consumer configuration
↓
Scale workers up/down
↓
Monitor consumer status
```

## 🛠️ Configuration

The system is configured via `configs/config.yaml`:

```yaml
server:
  port: 8080
  host: "0.0.0.0"

database:
  host: "postgres"
  port: 5432
  user: "postgres"
  password: "password"
  dbname: "multitenant_messaging"

rabbitmq:
  host: "rabbitmq"
  port: 5672
  user: "guest"
  password: "guest"

consumer:
  default_workers: 3
  max_workers: 10
  retry_attempts: 3
  retry_delay: 5s

metrics:
  enabled: true
  path: "/metrics"
```

## 🐳 Quick Start

### Prerequisites
- Docker and Docker Compose
- Go 1.21+ (for development)

### Development Setup
```bash
# Clone the repository
git clone <repository-url>
cd multitenant-messaging

# Start all services
docker-compose up -d

# Run database migrations
make migrate-up

# Access the application
# API: http://localhost:8080
# RabbitMQ Management: http://localhost:15672
# Grafana: http://localhost:3000 (admin/admin)
# Prometheus: http://localhost:9090
```

### API Usage

1. **Create a Tenant**
```bash
curl -X POST http://localhost:8080/api/v1/tenants \
  -H "Content-Type: application/json" \
  -d '{"name": "tenant-1"}'
```

2. **Send a Message**
```bash
curl -X POST http://localhost:8080/api/v1/tenants/1/messages \
  -H "Content-Type: application/json" \
  -d '{"queue_name": "orders", "message_body": "process order 123"}'
```

3. **Configure Consumer**
```bash
curl -X PUT http://localhost:8080/api/v1/tenants/1/consumers/orders/config \
  -H "Content-Type: application/json" \
  -d '{"workers": 5}'
```

## 📊 Monitoring

### Metrics Endpoints
- **Health Check**: `GET /health`
- **Prometheus Metrics**: `GET /metrics`
- **Consumer Status**: `GET /api/v1/consumers/status`

### Key Metrics
- Message processing rate
- Consumer worker count
- Error rates
- Processing latency
- Queue depths

## 🔒 Security

- **JWT Authentication**: Secure API access
- **Tenant Isolation**: Data and queue separation
- **Input Validation**: Request validation and sanitization
- **CORS Configuration**: Cross-origin request handling

## 🧪 Testing

```bash
# Run unit tests
make test

# Run integration tests
make test-integration

# Run with coverage
make test-coverage
```