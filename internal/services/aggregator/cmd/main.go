package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/LeonardoBeccarini/sdcc_project/internal/services/aggregator"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
)

func env(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}
func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}
func envDur(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func main() {
	host := env("RABBITMQ_HOST", "localhost")
	port := envInt("RABBITMQ_PORT", 1883)
	user := env("RABBITMQ_USER", "guest")
	pass := env("RABBITMQ_PASSWORD", "guest")
	exchange := env("RABBITMQ_EXCHANGE", "sensor_data")
	clientID := env("MQTT_CLIENT_ID", "aggregator-"+strconv.FormatInt(time.Now().Unix(), 10))

	// Questo nodo edge aggrega un solo field (es. field1 o field2)
	topicsCSV := env("SENSOR_TOPICS", "sensor/data/field1/+")
	var topics []string
	for _, t := range strings.Split(topicsCSV, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			topics = append(topics, t)
		}
	}
	if len(topics) == 0 {
		topics = []string{"sensor/data/field1/+"}
	}

	window := envDur("AGGREGATION_WINDOW", 15*time.Minute)

	ctx := context.Background()
	cfg := &rabbitmq.RabbitMQConfig{Host: host, Port: port, User: user, Password: pass, Exchange: exchange, ClientID: clientID}
	client, err := rabbitmq.NewRabbitMQConn(cfg, ctx)
	if err != nil {
		log.Fatalf("MQTT connect: %v", err)
	}

	// Publisher base, il servizio pubblicherà su sensor/aggregated/{field}/{sensor}
	publisher := rabbitmq.NewPublisher(client, "sensor/aggregated", cfg.Exchange)
	consumer := rabbitmq.NewMultiConsumer(client, topics, cfg.Exchange, nil)

	service := aggregator.NewDataAggregatorService(consumer, publisher, window)
	log.Printf("Data Aggregator running. subs=%v window=%s", topics, window)
	service.Start(ctx)
}
