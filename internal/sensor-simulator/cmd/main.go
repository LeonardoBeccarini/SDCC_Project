// cmd/sensor-sim/main.go
package main

import (
	"context"
	"flag"
	"log"
	"time"

	sensorSimulator "github.com/LeonardoBeccarini/sdcc_project/internal/sensor-simulator"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
)

func main() {
	// define flags
	sensorID := flag.String("sensor-id", "sensor1", "unique sensor identifier")
	fieldID := flag.String("field-id", "field1", "unique field identifier")
	clientID := flag.String("client-id", "sensorPublisher1", "MQTT client ID")
	interval := flag.Duration("interval", 10*time.Second, "publish interval")
	flag.Parse()

	// inject flags into config
	cfg := &rabbitmq.RabbitMQConfig{
		Host:     "localhost",
		Port:     1883,
		User:     "guest",
		Password: "guest",
		ClientID: *clientID,
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
	service := sensorSimulator.NewDataGenerator(publisher)

	// pass sensorID through context or modify StartPublishing to accept it
	service.StartPublishing(ctx, *interval, *sensorID, *fieldID)
}
