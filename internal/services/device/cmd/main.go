package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/LeonardoBeccarini/sdcc_project/internal/model/entities"
	"github.com/LeonardoBeccarini/sdcc_project/internal/services/device"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"

	pb "github.com/LeonardoBeccarini/sdcc_project/deploy/gen/go/irrigation"
	"google.golang.org/grpc"
)

func main() {
	fields := map[string]entities.Field{
		"field1": {
			ID:       "field1",
			CropType: "corn",
			Sensors: []entities.Sensor{
				{ID: "sensor1", FlowRate: 10.0},
				{ID: "sensor2", FlowRate: 15.0},
				{ID: "sensor3", FlowRate: 20.5},
			},
		},
		"field2": {
			ID:       "field2",
			CropType: "wheat",
			Sensors: []entities.Sensor{
				{ID: "sensor4", FlowRate: 5.5},
				{ID: "sensor5", FlowRate: 8.0},
				{ID: "sensor6", FlowRate: 10.0},
			},
		},
	}

	// Legge le variabili d'ambiente
	host := os.Getenv("RABBITMQ_HOST")
	if host == "" {
		host = "localhost"
	}
	portStr := os.Getenv("RABBITMQ_PORT")
	if portStr == "" {
		portStr = "1883"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		log.Fatalf("Invalid RABBITMQ_PORT: %v", err)
	}

	user := os.Getenv("RABBITMQ_USER")
	if user == "" {
		user = "guest"
	}
	password := os.Getenv("RABBITMQ_PASSWORD")
	if password == "" {
		password = "guest"
	}

	clientID := fmt.Sprintf("DeviceService-%s", os.Getenv("HOSTNAME"))
	cfg := &rabbitmq.RabbitMQConfig{
		Host:     host,
		Port:     port,
		User:     user,
		Password: password,
		ClientID: clientID,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client, err := rabbitmq.NewRabbitMQConn(cfg, ctx)
	if err != nil {
		log.Fatalf("Failed to connect to MQTT broker: %v", err)
	}

	consumer := rabbitmq.NewConsumer(client, "sensor/data", "sensor_data", nil)
	publisher := rabbitmq.NewPublisher(client, "event/stateChangeEvent", "device_commands")

	// Avvio DeviceService logica MQTT
	svc := device.NewDeviceService(consumer, publisher, fields)
	go func() {
		log.Println("Starting DeviceService MQTT loop...")
		svc.Start(ctx)
	}()

	// Avvio server gRPC
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterDeviceServiceServer(grpcServer, device.NewGrpcHandler(publisher, fields))

	log.Println("DeviceService gRPC server listening on :50051")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve gRPC: %v", err)
	}
}
