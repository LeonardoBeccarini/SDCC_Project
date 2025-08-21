module github.com/LeonardoBeccarini/sdcc_project

go 1.23.0

toolchain go1.23.4

replace github.com/LeonardoBeccarini/sdcc_project/internal/services/aggregator => ./internal/services/aggregator

require (
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/eclipse/paho.mqtt.golang v1.5.0
	github.com/joho/godotenv v1.5.1
	github.com/sony/gobreaker v1.0.0
	google.golang.org/grpc v1.74.2
	google.golang.org/protobuf v1.36.6
)

require (
	github.com/gorilla/websocket v1.5.3 // indirect
	golang.org/x/net v0.40.0 // indirect
	golang.org/x/sync v0.14.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.25.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250528174236-200df99c418a // indirect
)
