// internal/services/device/cmd/main.go
package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	pb "github.com/LeonardoBeccarini/sdcc_project/grpc/gen/go/irrigation"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
	"github.com/LeonardoBeccarini/sdcc_project/internal/services/device"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
	"google.golang.org/grpc"
)

func mustEnv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	if def != "" {
		return def
	}
	log.Fatalf("missing required env %s", k)
	return ""
}

func main() {
	// ---- ENV ----
	host := mustEnv("RABBITMQ_HOST", "")
	portStr := mustEnv("RABBITMQ_PORT", "1883")
	user := mustEnv("RABBITMQ_USER", "")
	pass := mustEnv("RABBITMQ_PASSWORD", "")
	clientID := mustEnv("RABBITMQ_CLIENTID", "device-service")
	grpcPort := mustEnv("GRPC_PORT", "50051")
	sensorsPath := mustEnv("SENSORS_CONFIG_PATH", "/app/config/sensors-config.json")

	// Topics / templates
	stateTopicTmpl := mustEnv("EVENT_STATECHANGE_TEMPLATE", "event/StateChange/{field}/{sensor}")
	resultTopicTmpl := mustEnv("IRRIGATION_RESULT_TOPIC_TMPL", "event/irrigationResult/{field}/{sensor}")

	// Liveness (heartbeat) config
	sensorDataSub := mustEnv("SENSOR_DATA_SUB", "sensor/data/+/+")
	livenessTTLStr := mustEnv("SENSOR_LIVENESS_TTL", "60s")
	offlineGraceStr := mustEnv("OFFLINE_GRACE", "5s")

	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil {
		log.Fatalf("invalid RABBITMQ_PORT: %v", err)
	}

	// ---- Load sensors: map field -> []Sensor -> map field -> Field{Sensors}
	raw, err := os.ReadFile(sensorsPath)
	if err != nil {
		log.Fatalf("read sensors config: %v", err)
	}
	var cfg map[string][]model.Sensor // {"field_1":[...]}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		log.Fatalf("unmarshal sensors config: %v", err)
	}
	fields := make(map[string]model.Field, len(cfg))
	for fid, list := range cfg {
		fields[fid] = model.Field{ID: fid, Sensors: list}
	}

	// ---- MQTT (RabbitMQ topic)
	rmqc := &rabbitmq.RabbitMQConfig{
		Host:     host,
		Port:     port,
		User:     user,
		Password: pass,
		ClientID: clientID,
		Kind:     "topic",
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := rabbitmq.NewRabbitMQConn(rmqc, ctx)
	if err != nil {
		log.Fatalf("MQTT connect error: %v", err)
	}

	// Publisher factory (fits device.GrpcHandler expectation)
	publisherFactory := func(topic string) rabbitmq.IPublisher {
		return rabbitmq.NewPublisher(client, topic)
	}

	// ---- Build handler
	handler := device.NewGrpcHandler(publisherFactory, stateTopicTmpl, fields)
	handler.SetResultTopicTemplate(resultTopicTmpl)

	// Parse liveness durations
	lttl, err := time.ParseDuration(livenessTTLStr)
	if err != nil {
		lttl = 60 * time.Second
	}
	grace, err := time.ParseDuration(offlineGraceStr)
	if err != nil {
		grace = 5 * time.Second
	}
	handler.SetLiveness(lttl, grace)

	// Subscribe to sensor heartbeat (implicit)
	dataConsumer := rabbitmq.NewConsumer(client, sensorDataSub, nil)
	dataConsumer.SetHandler(handler.OnSensorData)
	go dataConsumer.ConsumeMessage(ctx)

	// ---- gRPC server
	addr := ":" + grpcPort
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen %s: %v", addr, err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterDeviceServiceServer(grpcServer, handler)

	go func() {
		log.Printf("DeviceService gRPC %s; StateChange='%s'; Result='%s'; Liveness sub='%s' TTL=%s Grace=%s",
			addr, stateTopicTmpl, resultTopicTmpl, sensorDataSub, lttl, grace)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("gRPC serve error: %v", err)
		}
	}()

	// ---- graceful shutdown
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	<-sigc
	log.Println("shutting down...")
	grpcServer.GracefulStop()
	cancel()
	time.Sleep(300 * time.Millisecond)
}
