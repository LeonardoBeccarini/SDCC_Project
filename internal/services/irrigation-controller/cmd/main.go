package main

import (
	"context"
	"encoding/json"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model/messages"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/joho/godotenv"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

func main() {
	err := godotenv.Load("internal/config/rabbitmq.env")
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
	portStr := os.Getenv("RABBITMQ_PORT")
	if portStr == "" {
		// Try to fetch MQTT service NodePort
		out, err := exec.Command("kubectl", "get", "svc", "rabbitmq-mqtt", "-n", "rabbitmq", "-o", "jsonpath={.spec.ports[0].nodePort}").Output()
		if err != nil {
			log.Fatalf("Could not detect NodePort: %v", err)
		}
		portStr = string(out)
	}

	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil {
		log.Fatalf("Invalid RABBITMQ_PORT: %v", err)
	}
	// RabbitMQ MQTT configuration
	cfg := &rabbitmq.RabbitMQConfig{
		Host:     os.Getenv("RABBITMQ_HOST"),
		Port:     port,
		User:     os.Getenv("RABBITMQ_USER"),
		Password: os.Getenv("RABBITMQ_PASSWORD"),
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
		var aggregatedData messages.SensorData
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
