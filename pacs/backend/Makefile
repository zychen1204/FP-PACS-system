.PHONY: help build run clean test dev

help:
	@echo "PACS Backend - Available Commands:"
	@echo ""
	@echo "  make build       - Build the binary (pacs-backend.exe)"
	@echo "  make run         - Run the server (development mode)"
	@echo "  make dev         - Run with hot-reload (requires air)"
	@echo "  make clean       - Remove build artifacts"
	@echo "  make test        - Run tests"
	@echo "  make deps        - Download dependencies"
	@echo ""

deps:
	@echo "📥 Downloading Go dependencies..."
	go mod download
	go mod tidy
	@echo "✓ Dependencies downloaded"

build: deps
	@echo "🔨 Building PACS Backend..."
	go build -o pacs-backend.exe main.go
	@echo "✓ Build complete: pacs-backend.exe"

run: deps
	@echo "🚀 Starting PACS Backend Server (Go)..."
	@echo ""
	@echo "✓ Access API:       http://localhost:8080/v1/swipe"
	@echo "✓ Reporting API:    http://localhost:8081/v1/reports/attendance"
	@echo "✓ Health Check:     http://localhost:8080/healthz"
	@echo "✓ Metrics:          http://localhost:8080/metrics"
	@echo ""
	@echo "Press Ctrl+C to stop"
	@echo ""
	go run main.go

dev: deps
	@echo "🔥 Starting PACS Backend Server (Development mode with hot-reload)..."
	air

test:
	@echo "🧪 Running tests..."
	go test -v ./...

clean:
	@echo "🧹 Cleaning up..."
	rm -f pacs-backend.exe
	rm -f pacs-backend
	@echo "✓ Clean complete"

# Build for different platforms
build-linux:
	@echo "🔨 Building for Linux..."
	GOOS=linux GOARCH=amd64 go build -o pacs-backend main.go
	@echo "✓ Build complete: pacs-backend"

build-macos:
	@echo "🔨 Building for macOS..."
	GOOS=darwin GOARCH=amd64 go build -o pacs-backend main.go
	@echo "✓ Build complete: pacs-backend"

build-windows:
	@echo "🔨 Building for Windows..."
	GOOS=windows GOARCH=amd64 go build -o pacs-backend.exe main.go
	@echo "✓ Build complete: pacs-backend.exe"

# Docker operations
docker-build:
	@echo "🐳 Building Docker image..."
	docker build -t pacs-backend:latest .

docker-run:
	@echo "🐳 Running in Docker..."
	docker run -p 8080:8080 -p 8081:8081 pacs-backend:latest

.DEFAULT_GOAL := help
