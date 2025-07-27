package main

import (
	"context"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model/entities"
	"github.com/LeonardoBeccarini/sdcc_project/internal/services/device"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
	"log"
	"os/signal"
	"syscall"
)

func main() {
	// Define your fields with their sensors
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

	// RabbitMQ configuration for MQTT connection
	cfg := &rabbitmq.RabbitMQConfig{
		Host:     "localhost",
		Port:     1883,
		User:     "guest",
		Password: "guest",
		ClientID: "DeviceService1",
	}

	// Create context with cancellation on OS signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create MQTT connection
	client, err := rabbitmq.NewRabbitMQConn(cfg, ctx)
	if err != nil {
		log.Fatalf("Failed to connect to MQTT broker: %v", err)
	}

	// Create consumer to subscribe to sensor_data exchange for sensor readings
	consumer := rabbitmq.NewConsumer(client, "sensor/data", "sensor_data", nil)

	// Create publisher to publish device commands or state changes to device_commands exchange
	publisher := rabbitmq.NewPublisher(client, "event/stateChangeEvent", "device_commands")

	// Instantiate and start your DeviceService
	svc := device.NewDeviceService(consumer, publisher, fields)

	log.Println("Starting DeviceService...")
	svc.Start(ctx) // blocks until ctx canceled
	log.Println("DeviceService has shut down")
}
