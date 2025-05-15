package main

import (
	"context"
	sensorSimulator "github.com/LeonardoBeccarini/sdcc_project/internal/sensor-simulator"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
	"log"
	"time"
)

func main() {
	// RabbitMQ configuration for MQTT connection
	cfg := &rabbitmq.RabbitMQConfig{
		Host:     "localhost",
		Port:     1883, // Default port for MQTT
		User:     "guest",
		Password: "guest",
		ClientID: "sensorPublisher1", // Dynamic client IDs for multiple instances
		Exchange: "sensor_data",
		Kind:     "topic",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := rabbitmq.NewRabbitMQConn(cfg, ctx)
	if err != nil {
		log.Fatal(err)
	}

	publisher := rabbitmq.NewPublisher(client, "sensor/data", cfg.Exchange)
	service := sensorSimulator.NewSensorService(publisher)

	service.StartPublishing(ctx, 10*time.Second)
}
