# Stage 1: Build
FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build argument to select which service to compile
ARG SERVICE=access-api
RUN CGO_ENABLED=0 GOOS=linux go build -o /service ./cmd/${SERVICE}

# Stage 2: Runtime (Distroless for security)
FROM gcr.io/distroless/static-debian12:latest

COPY --from=builder /service /service

EXPOSE 8080 8081 8082

ENTRYPOINT ["/service"]
