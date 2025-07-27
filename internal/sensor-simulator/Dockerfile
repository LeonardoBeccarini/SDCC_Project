# Use official Golang image to build the Go application
FROM golang:1.23-alpine AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy the Go Modules manifests
COPY go.mod go.sum ./

# Download the Go dependencies. Dependencies will be cached if the go.mod and go.sum are not changed

# Copy the source code into the container
# Copy the entire internal directory and files
# Copia i file sorgente
COPY ./internal/sensor-simulator /app/internal/sensor-simulator
COPY ./internal/model /app/internal/model
COPY ./internal/config /app/internal/config
COPY ./deploy /app/deploy
COPY ./pkg/rabbitmq /app/pkg/rabbitmq


# Esegui go mod tidy per gestire le dipendenze
RUN go mod tidy

# Esegui la build
RUN go build -o /app/sensor-simulator /app/internal/sensor-simulator/cmd/main.go

# Expose port (optional, depending on your configuration)
EXPOSE 8080

# Run the Go app
CMD ["/app/sensor-simulator"]



