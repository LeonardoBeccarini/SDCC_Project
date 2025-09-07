# ==== builder ====
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /src

# Mod cache
COPY go.mod go.sum ./
RUN go mod download

# Sorgenti necessari
COPY internal /src/internal
COPY pkg /src/pkg

# Build binario del servizio persistence
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o /out/persistence ./internal/services/persistence/cmd

# ==== runtime ====
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/persistence /app/persistence

# Porta HTTP usata dal gateway
EXPOSE 8080

ENTRYPOINT ["/app/persistence"]
