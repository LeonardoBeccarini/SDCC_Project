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
	exchange := mustEnv("RABBITMQ_EXCHANGE", "sensor_data")

	topicTmpl := mustEnv("EVENT_STATECHANGE_TEMPLATE", "event/StateChange/{field}/{sensor}")

	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil {
		log.Fatalf("invalid RABBITMQ_PORT: %v", err)
	}

	// ---- Carica sensori: mappa field -> sensori ----
	raw, err := os.ReadFile(sensorsPath)
	if err != nil {
		log.Fatalf("read sensors config: %v", err)
	}
	var cfg map[string][]model.Sensor // {"field_1":[...]}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		log.Fatalf("unmarshal sensors config: %v", err)
	}
	fields := make(map[string]model.Field)
	for fid, list := range cfg {
		fields[fid] = model.Field{ID: fid, Sensors: list}
	}

	// ---- MQTT (RabbitMQ con exchange "topic") ----
	rmqc := &rabbitmq.RabbitMQConfig{
		Host:     host,
		Port:     port,
		User:     user,
		Password: pass,
		ClientID: clientID,
		Exchange: exchange,
		Kind:     "topic",
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := rabbitmq.NewRabbitMQConn(rmqc, ctx)
	if err != nil {
		log.Fatalf("MQTT connect error: %v", err)
	}

	// factory per creare un publisher col topic calcolato
	publisherFactory := func(topic string) rabbitmq.IPublisher {
		return rabbitmq.NewPublisher(client, topic, rmqc.Exchange)
	}

	// ---- gRPC server ----
	addr := ":" + grpcPort
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen %s: %v", addr, err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterDeviceServiceServer(
		grpcServer,
		device.NewGrpcHandler(publisherFactory, topicTmpl, fields),
	)

	go func() {
		log.Printf("DeviceService gRPC %s; MQTT exchange '%s'; topics template '%s'",
			addr, rmqc.Exchange, topicTmpl)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("gRPC serve error: %v", err)
		}
	}()

	// ---- graceful shutdown ----
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	<-sigc
	log.Println("shutting down...")
	cancel()
	time.Sleep(300 * time.Millisecond)
}
