# ==== builder ====
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /src

# Mod cache (contesto = root repo!)
COPY go.mod go.sum ./
RUN go mod download

# Sorgenti usati dall'aggregator
COPY internal /src/internal
COPY pkg /src/pkg

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o /out/data-aggregator ./internal/services/aggregator/cmd

# ==== runtime ====
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/data-aggregator /app/data-aggregator
ENTRYPOINT ["/app/data-aggregator"]
