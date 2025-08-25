module github.com/LeonardoBeccarini/sdcc_project

go 1.23.0

toolchain go1.23.4

replace github.com/LeonardoBeccarini/sdcc_project/internal/services/aggregator => ./internal/services/aggregator

require (
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/eclipse/paho.mqtt.golang v1.5.0
	github.com/influxdata/influxdb-client-go/v2 v2.14.0
	github.com/sony/gobreaker v1.0.0
	google.golang.org/grpc v1.74.2
	google.golang.org/protobuf v1.36.8
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/influxdata/line-protocol v0.0.0-20210922203350-b1ad95c89adf // indirect
	github.com/oapi-codegen/runtime v1.0.0 // indirect
	golang.org/x/net v0.40.0 // indirect
	golang.org/x/sync v0.14.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.25.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250818200422-3122310a409c // indirect

)
