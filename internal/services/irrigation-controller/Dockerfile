########################
# 🔨 Builder
########################
ARG GO_VERSION=1.23.4
FROM golang:${GO_VERSION}-bookworm AS builder

# Versioni pin-ate
ARG PROTOC_VERSION=21.12
ARG PROTOC_ZIP=protoc-${PROTOC_VERSION}-linux-x86_64.zip
ARG PROTOC_GEN_GO_VERSION=v1.36.8
ARG PROTOC_GEN_GO_GRPC_VERSION=v1.5.1

# Dipendenze
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl unzip ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Installa protoc
WORKDIR /tmp
RUN curl -sSLo ${PROTOC_ZIP} https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/${PROTOC_ZIP} \
 && unzip -q ${PROTOC_ZIP} bin/protoc 'include/*' -d /usr/local \
 && rm -f ${PROTOC_ZIP}

# Installa i plugin Go
ENV GOBIN=/go/bin
ENV PATH="${GOBIN}:${PATH}"
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION} \
 && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GEN_GO_GRPC_VERSION}

# Mod cache per velocizzare
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

# Codice
COPY internal /src/internal
COPY pkg      /src/pkg
COPY grpc     /src/grpc

RUN mkdir -p grpc/gen/go/irrigation && \
    protoc -I grpc/proto \
      --go_out=grpc/gen/go/irrigation --go_opt=paths=source_relative \
      --go-grpc_out=grpc/gen/go/irrigation --go-grpc_opt=paths=source_relative \
      grpc/proto/irrigation.proto && \
    test -s grpc/gen/go/irrigation/irrigation.pb.go && \
    test -s grpc/gen/go/irrigation/irrigation_grpc.pb.go

# Build binario
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/irrigation-controller ./internal/services/irrigation-controller/cmd

########################
# 🚀 Runtime
########################
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/irrigation-controller /irrigation-controller
USER nonroot:nonroot
ENTRYPOINT ["/irrigation-controller"]



