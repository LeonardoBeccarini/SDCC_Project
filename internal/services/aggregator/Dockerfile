# Use official Golang image to build the Go application
FROM golang:1.23-alpine AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy the Go Modules manifests
COPY go.mod go.sum ./

# Copia i file sorgente
COPY ./internal/services/aggregator /app/internal/services/aggregator
COPY ./internal/model /app/internal/model
COPY ./internal/config /app/internal/config
COPY ./deploy /app/deploy
COPY ./pkg/rabbitmq /app/pkg/rabbitmq

# Esegui go mod tidy per gestire le dipendenze
RUN go mod tidy

# Esegui la build
RUN go build -o /app/aggregator /app/internal/services/aggregator/cmd/main.go

# Expose port (optional, depending on your configuration)
#EXPOSE 8080

# Run the Go app
CMD ["/app/aggregator"]