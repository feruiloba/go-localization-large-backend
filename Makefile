.PHONY: help build run dev test clean docker-build docker-up docker-down docker-logs docker-restart load-test-normal load-test-saturation load-test-allocation

# Default target
help:
	@echo "Available targets:"
	@echo "  make build          - Build the Go application"
	@echo "  make run            - Run the application locally"
	@echo "  make run-limited    - Run with limited concurrent connections"
	@echo "  make dev            - Run with hot reload (requires air)"
	@echo "  make test           - Run tests"
	@echo "  make clean          - Clean build artifacts"
	@echo ""
	@echo "Docker commands:"
	@echo "  make docker-build   - Build Docker image"
	@echo "  make docker-up      - Start Docker containers"
	@echo "  make docker-down    - Stop Docker containers"
	@echo "  make docker-logs    - View Docker logs"
	@echo "  make docker-restart - Restart Docker containers"
	@echo "  make up             - Alias for docker-up"
	@echo "  make down           - Alias for docker-down"
	@echo ""
	@echo "Load testing:"
	@echo "  make load-test-normal     - Run normal load test (all fast clients)"
	@echo "  make load-test-saturation - Run saturation load test (checking if fast clients stay fast)"
	@echo "  make load-test-allocation - Run A/B allocation consistency test"

# Build the application
build:
	@echo "Building application..."
	go build -o bin/main main.go

# Run the application locally
run:
	@echo "Running application..."
	go run main.go

# Run with limited concurrent connections (for testing)
run-limited:
	@echo "Running application with limited connections (max 20)..."
	MAX_CONNS=20 go run main.go

# Run with hot reload (requires air: go install github.com/cosmtrek/air@latest)
dev:
	@echo "Starting development server with hot reload..."
	air

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	go clean

# Docker commands
docker-build:
	@echo "Building Docker image..."
	docker-compose build

docker-up:
	@echo "Starting Docker containers..."
	docker-compose up -d
	@echo "Server is running at http://localhost:3000"
	@echo "Health check: curl http://localhost:3000/health"

docker-down:
	@echo "Stopping Docker containers..."
	docker-compose down

docker-logs:
	@echo "Viewing Docker logs..."
	docker-compose logs -f

docker-restart:
	@echo "Restarting Docker containers..."
	docker-compose restart

# Aliases for convenience
up: docker-up
down: docker-down

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod tidy
	go mod download

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Run linter
lint:
	@echo "Running linter..."
	golangci-lint run

# Load testing commands
load-test-normal:
	@echo "Running normal load test (all fast clients)..."
	@echo "Make sure the server is running (make up or make run)"
	@sleep 2
	go run cmd/loadtest/main.go -mode normal -fast 20 -requests 100 -duration 30s

load-test-saturation:
	@echo "Running saturation load test (checking if fast clients stay fast)..."
	@echo "This test floods the server with slow clients and measures fast client latency."
	@echo "Make sure the server is running (make up or make run)"
	@sleep 2
	go run cmd/loadtest/main.go -mode saturation -duration 60s

# Build load test binary
build-load-test:
	@echo "Building load test binary..."
	go build -o bin/loadtest cmd/loadtest/main.go

# A/B allocation consistency test
load-test-allocation:
	@echo "Running A/B allocation consistency test..."
	@echo "This test verifies that each user always receives the same payload."
	@echo "Make sure the server is running (make up or make run)"
	@sleep 2
	go run cmd/allocationtest/main.go -users 100 -requests 5 -concurrency 10 -output allocation_test_results.md

