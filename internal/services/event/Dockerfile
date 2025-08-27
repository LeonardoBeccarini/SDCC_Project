# ==== builder ====
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /src

# Mod cache (context = repo root)
COPY go.mod go.sum ./
RUN go mod download

# Sources needed by event service
COPY internal /src/internal
COPY pkg /src/pkg

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o /out/event-service ./internal/services/event/cmd

# ==== runtime ====
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/event-service /app/event-service
EXPOSE 8080
ENTRYPOINT ["/app/event-service"]
