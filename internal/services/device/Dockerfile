# Stage 1: build
FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build DeviceService
RUN CGO_ENABLED=0 GOOS=linux go build -o device-service ./internal/services/device/cmd

# Stage 2: runtime minimale
FROM alpine:3.19
WORKDIR /root/
COPY --from=builder /app/device-service .
EXPOSE 50051
ENTRYPOINT ["./device-service"]
