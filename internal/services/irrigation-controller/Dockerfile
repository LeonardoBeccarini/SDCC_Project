# ========= Builder =========
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /src

# Mod cache
COPY go.mod go.sum ./
RUN go mod download

# Sorgenti (serve anche grpc/ per i .pb.go)
COPY internal /src/internal
COPY pkg      /src/pkg
COPY grpc     /src/grpc

# Build binario
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /out/irrigation-controller ./internal/services/irrigation-controller/cmd

# ========= Runtime =========
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/irrigation-controller /app/irrigation-controller
ENTRYPOINT ["/app/irrigation-controller"]


