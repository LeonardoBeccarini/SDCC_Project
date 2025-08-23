# ==== builder ====
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /src

# Mod cache
COPY go.mod go.sum ./
RUN go mod download

# Sorgenti
COPY internal /src/internal
COPY pkg      /src/pkg
COPY grpc     /src/grpc

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /out/device-service ./internal/services/device/cmd

# ==== runtime ====
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/device-service /app/device-service
EXPOSE 50051
ENTRYPOINT ["/app/device-service"]



