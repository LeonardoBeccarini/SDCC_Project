# Use official Golang image to build the Go application
FROM golang:1.23-alpine AS builder

# Install git and certificates (needed for go mod tidy)
RUN apk add --no-cache git ca-certificates

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy the Go Modules manifests
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy all source code needed
COPY ./internal/sensor-simulator /app/internal/sensor-simulator
COPY ./internal/model /app/internal/model
COPY ./internal/config /app/internal/config
COPY ./internal/services/aggregator /app/internal/services/aggregator
COPY ./deploy /app/deploy
COPY ./pkg/rabbitmq /app/pkg/rabbitmq

# Run go mod tidy (now aggregator exists)
RUN go mod tidy

# Build the Go app
RUN go build -o /app/sensor-simulator /app/internal/sensor-simulator/cmd/main.go

# Expose port (optional)
EXPOSE 8080

# Run the Go app
CMD ["/app/sensor-simulator"]



