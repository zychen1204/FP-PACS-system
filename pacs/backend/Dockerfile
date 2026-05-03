# Multi-stage build for minimal image
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy source code
COPY . .

# Download dependencies and build
RUN go mod download && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o pacs-backend main.go

# Runtime stage - using distroless for minimal attack surface
FROM alpine:latest

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/pacs-backend .

# Expose ports
EXPOSE 8080 8081

# Health check
HEALTHCHECK --interval=10s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/healthz || exit 1

# Run the application
CMD ["./pacs-backend"]
