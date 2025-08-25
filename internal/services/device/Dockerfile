# ==== builder ====
FROM golang:1.23-alpine AS builder
WORKDIR /src
RUN apk add --no-cache git ca-certificates protobuf

# >>> versioni allineate al runtime <<<
ARG PROTOBUF_GO_VERSION=v1.36.8
ARG GRPC_GO_PLUGIN_VERSION=v1.5.1
ENV PATH="/go/bin:${PATH}"

RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOBUF_GO_VERSION} && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@${GRPC_GO_PLUGIN_VERSION}

COPY go.mod go.sum ./
RUN go mod download

COPY internal /src/internal
COPY pkg      /src/pkg
COPY grpc     /src/grpc


# Rigenerazione protobuf: forza l'output dentro grpc/gen/go/irrigation
RUN rm -rf /src/grpc/gen/go && mkdir -p /src/grpc/gen/go/irrigation && \
    protoc -I /src/grpc/proto \
      --go_out=/src/grpc/gen/go/irrigation --go_opt=paths=source_relative \
      --go-grpc_out=/src/grpc/gen/go/irrigation --go-grpc_opt=paths=source_relative \
      $(find /src/grpc/proto -name "*.proto" -print)

ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -ldflags="-s -w" -o /out/device-service ./internal/services/device/cmd

# ==== runtime ====
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/device-service /app/device-service
EXPOSE 50051
ENTRYPOINT ["/app/device-service"]





