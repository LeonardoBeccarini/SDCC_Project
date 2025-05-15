package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	// RabbitMQ MQTT configuration
	cfg := &rabbitmq.RabbitMQConfig{
		Host:     "localhost",
		Port:     1883,
		User:     "guest",
		Password: "guest",
		ClientID: "dummyConsumer1",
		Exchange: "sensor_data",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup graceful shutdown on interrupt signals
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutdown signal received")
		cancel()
	}()

	// Connect to MQTT broker
	client, err := rabbitmq.NewRabbitMQConn(cfg, ctx)
	if err != nil {
		log.Fatalf("Failed to connect MQTT broker: %v", err)
	}

	// Handler function to process incoming aggregated data messages
	handler := func(queue string, message mqtt.Message) error {
		var aggregatedData model.SensorData
		err := json.Unmarshal(message.Payload(), &aggregatedData)
		if err != nil {
			log.Printf("Failed to unmarshal aggregated data: %v", err)
			return err
		}

		log.Printf("Received Aggregated Data: SensorID=%s, Moisture=%d, Timestamp=%s",
			aggregatedData.SensorID, aggregatedData.Moisture,
			aggregatedData.Timestamp.Format("2006-01-02 15:04:05"))

		return nil
	}

	// Create consumer subscribing to "sensor/aggregatedData" topic
	consumer := rabbitmq.NewConsumer(client, "sensor/aggregatedData", cfg.Exchange, handler)

	// Start consuming
	consumer.ConsumeMessage(nil)

	// Block until context cancellation
	<-ctx.Done()

	// Disconnect cleanly
	client.Disconnect(250)
	log.Println("Dummy consumer stopped")
}
