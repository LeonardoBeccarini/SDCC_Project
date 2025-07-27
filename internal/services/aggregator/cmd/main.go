package main

import (
	"context"
	"github.com/LeonardoBeccarini/sdcc_project/internal/services/aggregator"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

func mustGetenv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("Missing required environment variable: %s", key)
	}
	return val
}

func main() {
	host := mustGetenv("RABBITMQ_HOST")
	portStr := mustGetenv("RABBITMQ_PORT")
	user := mustGetenv("RABBITMQ_USER")
	pass := mustGetenv("RABBITMQ_PASSWORD")
	clientID := mustGetenv("RABBITMQ_CLIENTID")

	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil {
		log.Fatalf("Invalid RABBITMQ_PORT: %v", err)
	}

	// Use client ID to determine which topics to subscribe to
	var topics []string
	switch clientID {
	case "aggregator-field1":
		topics = []string{
			"field_1/sensor_1/data",
			"field_1/sensor_2/data",
		}
	case "aggregator-field2":
		topics = []string{
			"field_2/sensor_3/data",
			"field_2/sensor_4/data",
		}
	default:
		log.Fatalf("Unknown clientID: %s", clientID)
	}

	cfg := &rabbitmq.RabbitMQConfig{
		Host:     host,
		Port:     port,
		User:     user,
		Password: pass,
		ClientID: clientID,
		Exchange: "sensor_data",
		Kind:     "topic",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := rabbitmq.NewRabbitMQConn(cfg, ctx)
	if err != nil {
		log.Fatalf("Failed to connect to MQTT broker: %v", err)
	}

	publisher := rabbitmq.NewPublisher(client, "sensor/aggregatedData", cfg.Exchange)

	consumer := rabbitmq.NewMultiConsumer(client, topics, cfg.Exchange, nil)

	service := aggregator.NewDataAggregatorService(consumer, publisher, 30*time.Second)

	log.Printf("Data Aggregator [%s] is running...", clientID)
	service.Start(ctx)
}
