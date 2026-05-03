@echo off
REM PACS Backend Server - Startup Script for Windows

cd /d "%~dp0"

echo.
echo ╔════════════════════════════════════════════════════════╗
echo ║  PACS Backend Server (Go)                              ║
echo ║  Cloud-Native Physical Access Control System           ║
echo ╚════════════════════════════════════════════════════════╝
echo.

REM Check if Go is installed
go version >nul 2>&1
if errorlevel 1 (
    echo ❌ Go is not installed or not in PATH
    echo Please install Go from: https://golang.org/dl/
    echo.
    pause
    exit /b 1
)

echo ✓ Go is installed

REM Download dependencies
echo.
echo 📥 Downloading Go dependencies...
go mod download
if errorlevel 1 (
    echo ❌ Failed to download dependencies
    pause
    exit /b 1
)

echo ✓ Dependencies downloaded

REM Run the server
echo.
echo 🚀 Starting PACS Backend Server...
echo.
echo ✓ Access API:       http://localhost:8080/v1/swipe
echo ✓ Reporting API:    http://localhost:8081/v1/reports/attendance
echo ✓ Health Check:     http://localhost:8080/healthz
echo ✓ Metrics:          http://localhost:8080/metrics
echo.
echo Press Ctrl+C to stop the server
echo.

go run main.go

pause
