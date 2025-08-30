# ==== builder ====
FROM golang:1.23-alpine AS builder
WORKDIR /src

# Usa i mod del root del repo
COPY go.mod go.sum ./
RUN go mod download

# Codice app
COPY internal/ /src/internal/
# Se hai anche una cartella pkg/, sblocca la riga seguente:
# COPY pkg/ /src/pkg/

# Build gateway
WORKDIR /src/internal/services/gateway/cmd/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/gateway .

# ==== runtime ====
FROM alpine:3.20
# (facoltativo ma consigliato per HTTPS)
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/gateway /app/gateway

ENV PORT=5009
EXPOSE 5009

CMD ["/app/gateway"]
