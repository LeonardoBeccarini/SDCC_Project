FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Compila esplicitamente per Linux amd64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o irrigation-controller ./internal/services/irrigation-controller

FROM alpine:3.19
WORKDIR /root/
COPY --from=builder /app/irrigation-controller /root/irrigation-controller
RUN chmod +x /root/irrigation-controller
ENTRYPOINT ["/root/irrigation-controller"]
