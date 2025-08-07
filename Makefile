.PHONY: build run test clean docker-build docker-run migrate-up migrate-down swagger

# Build the application
build:
	go build -o bin/server cmd/server/main.go

# Run the application
run:
	go run cmd/server/main.go

# Run tests
test:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out

# Install dependencies
deps:
	go mod download
	go mod tidy

# Generate swagger documentation
swagger:
	swag init -g cmd/server/main.go -o docs

# Run database migrations
migrate-up:
	migrate -path configs/migrations -database "postgres://postgres:password@localhost:5432/multitenant_messaging?sslmode=disable" up

# Rollback database migrations
migrate-down:
	migrate -path configs/migrations -database "postgres://postgres:password@localhost:5432/multitenant_messaging?sslmode=disable" down

# Docker build
docker-build:
	docker build -t multitenant-messaging .

# Docker run
docker-run:
	docker run -p 8080:8080 multitenant-messaging

# Development setup
dev-setup:
	@echo "Setting up development environment..."
	@echo "Installing dependencies..."
	$(MAKE) deps
	@echo "Generating swagger docs..."
	$(MAKE) swagger
	@echo "Running migrations..."
	$(MAKE) migrate-up
	@echo "Development setup complete!"

# Lint code
lint:
	golangci-lint run

# Format code
fmt:
	go fmt ./...

# Vet code
vet:
	go vet ./... 

# Run integration tests
test-integration:
	go test ./test/integration/ -v -timeout 10m

# Run all tests
test-all:
	go test ./... -v -timeout 10m 