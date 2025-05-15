package main

import (
	"context"
	"github.com/LeonardoBeccarini/sdcc_project/internal/services/aggregator"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
	"log"
	"time"
)

func main() {
	// RabbitMQ configuration for MQTT connection
	cfg := &rabbitmq.RabbitMQConfig{
		Host:     "localhost",
		Port:     1883,
		User:     "guest",
		Password: "guest",
		ClientID: "dataAggregator1",
		Exchange: "sensor_data",
		Kind:     "topic",
	}

	// Create a context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Establish a shared MQTT connection using the config and context
	client, err := rabbitmq.NewRabbitMQConn(cfg, ctx)
	if err != nil {
		log.Fatalf("Failed to connect to MQTT broker: %v", err)
	}

	// Create the Publisher instance
	publisher := rabbitmq.NewPublisher(client, "sensor/aggregatedData", cfg.Exchange)

	// Create the Consumer instance, nil handler because it will be injected later
	consumer := rabbitmq.NewConsumer(client, "sensor/data", cfg.Exchange, nil)
	/*if err != nil {
		log.Fatalf("Failed to create Consumer: %v", err)
	}*/

	// Now, pass the dependencies (publisher and consumer) into the DataAggregatorService
	dataAggregatorService := aggregator.NewDataAggregatorService(consumer, publisher, 1*time.Minute)

	// Start the DataAggregatorService
	log.Println("Data Aggregator service is running...")
	dataAggregatorService.Start(ctx)
}
