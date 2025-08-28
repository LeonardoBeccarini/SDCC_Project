# ---- build ----
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . /src
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/gateway .

# ---- run ----
FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/gateway /app/gateway
EXPOSE 5009
ENTRYPOINT ["/app/gateway"]
