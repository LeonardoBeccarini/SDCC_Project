# ==== builder ====
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /src

# Mod cache
COPY go.mod go.sum ./
RUN go mod download

# Sorgenti necessari per il sensor-simulator
COPY internal /src/internal
COPY pkg /src/pkg

# Build (package directory: ./internal/sensor-simulator/cmd)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o /out/sensor-simulator ./internal/sensor-simulator/cmd

# ==== runtime ====
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/sensor-simulator /app/sensor-simulator
ENTRYPOINT ["/app/sensor-simulator"]




