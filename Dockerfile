# syntax=docker/dockerfile:1

# Step 1: Build the Go binary
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy dependency files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build argument to specify which service to build
ARG SERVICE=access-api
RUN CGO_ENABLED=0 GOOS=linux go build -o /pacs-service ./cmd/${SERVICE}

# Step 2: Create a tiny, secure image
# We use Google's Distroless image (static) which contains NO shell and is highly secure.
FROM gcr.io/distroless/static-debian12:latest

COPY --from=builder /pacs-service /pacs-service

# Explicitly use numeric UID for Kubernetes security context (RunAsNonRoot)
USER 65532:65532

ENTRYPOINT ["/pacs-service"]
