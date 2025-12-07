# Makefile for HTTP/2 Test Client

BINARY_NAME=http2-client
GO=go
BUILD_DIR=.
CMD_DIR=./cmd/http2-client

# Default target
.PHONY: all
all: build

# Build the client
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Build for multiple platforms
.PHONY: build-all
build-all: build-linux build-darwin build-windows
	@echo "All platform builds complete"

.PHONY: build-linux
build-linux:
	@echo "Building for Linux..."
	GOOS=linux GOARCH=amd64 $(GO) build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_DIR)

.PHONY: build-darwin
build-darwin:
	@echo "Building for macOS..."
	GOOS=darwin GOARCH=amd64 $(GO) build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_DIR)
	GOOS=darwin GOARCH=arm64 $(GO) build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_DIR)

.PHONY: build-windows
build-windows:
	@echo "Building for Windows..."
	GOOS=windows GOARCH=amd64 $(GO) build -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_DIR)

# Run tests
.PHONY: test
test: build
	@echo "Running basic HTTP/1.1 test..."
	./$(BINARY_NAME) -url http://localhost:8080 -n 10 -http2=false
	@echo ""
	@echo "Running HTTP/2 (H2C) test..."
	./$(BINARY_NAME) -url http://localhost:8080 -n 10 -http2 -h2c

# Quick test with verbose output
.PHONY: test-verbose
test-verbose: build
	@echo "Running verbose test..."
	./$(BINARY_NAME) -url http://localhost:8080 -n 5 -v

# Load test
.PHONY: load-test
load-test: build
	@echo "Running load test (100 requests, 10 concurrent)..."
	./$(BINARY_NAME) -url http://localhost:8080 -n 100 -c 10

# Heavy load test
.PHONY: stress-test
stress-test: build
	@echo "Running stress test (1000 requests, 50 concurrent)..."
	./$(BINARY_NAME) -url http://localhost:8080 -n 1000 -c 50

# Protocol comparison
.PHONY: compare
compare: build
	@echo "=== HTTP/1.1 Performance ==="
	./$(BINARY_NAME) -url http://localhost:8080 -n 100 -c 10 -http2=false
	@echo ""
	@echo "=== HTTP/2 Performance ==="
	./$(BINARY_NAME) -url http://localhost:8080 -n 100 -c 10 -http2 -h2c

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-*
	@echo "Clean complete"

# Install dependencies
.PHONY: deps
deps:
	@echo "Installing dependencies..."
	$(GO) mod download
	$(GO) mod tidy

# Run with default settings
.PHONY: run
run: build
	./$(BINARY_NAME)

# Run with custom URL
.PHONY: run-custom
run-custom: build
	./$(BINARY_NAME) -url $(URL) -n $(N) -c $(C)

# Docker build
.PHONY: docker-build
docker-build:
	@echo "Building Docker image..."
	cd ../.. && docker build -f test/Dockerfile.test-client -t http2-client .

# Docker run
.PHONY: docker-run
docker-run: docker-build
	@echo "Running Docker container..."
	docker run --network gateway-net http2-client -url http://gateway:8080 -n 100 -c 10

# Help
.PHONY: help
help:
	@echo "HTTP/2 Test Client - Makefile Commands"
	@echo ""
	@echo "Build Commands:"
	@echo "  make build         - Build the client for current platform"
	@echo "  make build-all     - Build for all platforms (Linux, macOS, Windows)"
	@echo "  make build-linux   - Build for Linux"
	@echo "  make build-darwin  - Build for macOS (amd64 and arm64)"
	@echo "  make build-windows - Build for Windows"
	@echo ""
	@echo "Test Commands:"
	@echo "  make test          - Run basic tests (HTTP/1.1 and HTTP/2)"
	@echo "  make test-verbose  - Run test with verbose output"
	@echo "  make load-test     - Run load test (100 req, 10 concurrent)"
	@echo "  make stress-test   - Run stress test (1000 req, 50 concurrent)"
	@echo "  make compare       - Compare HTTP/1.1 vs HTTP/2 performance"
	@echo ""
	@echo "Run Commands:"
	@echo "  make run           - Run with default settings"
	@echo "  make run-custom    - Run with custom settings (URL=... N=... C=...)"
	@echo ""
	@echo "Docker Commands:"
	@echo "  make docker-build  - Build Docker image"
	@echo "  make docker-run    - Run in Docker container"
	@echo ""
	@echo "Utility Commands:"
	@echo "  make deps          - Install/update dependencies"
	@echo "  make clean         - Remove build artifacts"
	@echo "  make help          - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make test"
	@echo "  make load-test"
	@echo "  make run-custom URL=http://localhost:8080 N=50 C=5"
	@echo "  make compare"
