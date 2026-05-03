#!/usr/bin/env bash
# PACS Backend startup script for Linux/macOS

set -e

cd "$(dirname "$0")"

echo ""
echo "╔════════════════════════════════════════════════════════╗"
echo "║  PACS Backend Server (Go)                              ║"
echo "║  Cloud-Native Physical Access Control System           ║"
echo "╚════════════════════════════════════════════════════════╝"
echo ""

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed"
    echo "Please install Go from: https://golang.org/dl/"
    exit 1
fi

echo "✓ Go is installed: $(go version)"
echo ""

# Download dependencies
echo "📥 Downloading Go dependencies..."
go mod download

echo "✓ Dependencies downloaded"
echo ""

# Run the server
echo "🚀 Starting PACS Backend Server..."
echo ""
echo "✓ Access API:       http://localhost:8080/v1/swipe"
echo "✓ Reporting API:    http://localhost:8081/v1/reports/attendance"
echo "✓ Health Check:     http://localhost:8080/healthz"
echo "✓ Metrics:          http://localhost:8080/metrics"
echo ""
echo "Press Ctrl+C to stop the server"
echo ""

go run main.go
