FROM golang:1.23-alpine AS builder

ENV GOOS=linux
ENV GOARCH=amd64
ENV CGO_ENABLED=0

WORKDIR /app

# Copia tutto il progetto
COPY . .

# Copia anche il file .env (se vuoi davvero usarlo nel container)
COPY internal/config/rabbitmq.env /app/internal/config/rabbitmq.env

# Risolve le dipendenze
RUN go mod tidy

# Compila il main dell’irrigation-controller
RUN go build -o irrigation-controller ./internal/services/irrigation-controller/cmd/main.go

FROM alpine:3.18
WORKDIR /root

COPY --from=builder /app/irrigation-controller .
# Copia il file .env anche nell'immagine finale (se serve)
COPY --from=builder /app/internal/config/rabbitmq.env ./internal/config/rabbitmq.env

CMD ["./irrigation-controller"]

