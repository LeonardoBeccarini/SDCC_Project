module github.com/LeonardoBeccarini/sdcc_project

go 1.23.0

toolchain go1.23.4

replace github.com/LeonardoBeccarini/sdcc_project/internal/services/aggregator => ./internal/services/aggregator

require (
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/eclipse/paho.mqtt.golang v1.5.0
	github.com/influxdata/influxdb-client-go/v2 v2.14.0
	github.com/prometheus/client_golang v1.23.2
	github.com/sony/gobreaker v1.0.0
	google.golang.org/grpc v1.74.2
	google.golang.org/protobuf v1.36.8
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/influxdata/line-protocol v0.0.0-20210922203350-b1ad95c89adf // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/oapi-codegen/runtime v1.0.0 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250818200422-3122310a409c // indirect

)
