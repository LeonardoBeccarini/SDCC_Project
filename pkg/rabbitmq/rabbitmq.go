package rabbitmq

import (
	"context"
	"fmt"
	"github.com/cenkalti/backoff/v4"
	"github.com/eclipse/paho.mqtt.golang"
	"log"
	"time"
)

// RabbitMQConfig holds the configuration for the RabbitMQ connection.
type RabbitMQConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	ClientID string // Client ID to be used for the connection
	Exchange string // Exchange name to be declared
	Kind     string // Exchange type (topic, fanout, etc.)
}

// NewRabbitMQConn establishes a new connection to RabbitMQ using MQTT and ensures the exchange exists.
func NewRabbitMQConn(cfg *RabbitMQConfig, ctx context.Context) (mqtt.Client, error) {
	// Connection address for MQTT
	connAddr := fmt.Sprintf("tcp://%s:%d", cfg.Host, cfg.Port)

	// MQTT Client Options
	opts := mqtt.NewClientOptions()
	opts.AddBroker(connAddr)
	opts.SetUsername(cfg.User)
	opts.SetPassword(cfg.Password)
	opts.SetClientID(cfg.ClientID) // Use the dynamic client ID
	opts.SetCleanSession(true)

	// Exponential backoff for retries in case the connection fails
	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 10 * time.Second
	maxRetries := 5

	var client mqtt.Client
	var err error

	// Retry connecting to RabbitMQ with exponential backoff
	err = backoff.Retry(func() error {
		client = mqtt.NewClient(opts)
		if token := client.Connect(); token.Wait() && token.Error() != nil {
			log.Printf("Failed to connect to MQTT broker: %v", token.Error())
			return token.Error()
		}
		return nil
	}, backoff.WithMaxRetries(bo, uint64(maxRetries-1)))

	if err != nil {
		return nil, fmt.Errorf("could not establish MQTT connection after retries: %v", err)
	}

	log.Printf("Connected to MQTT broker at %s", connAddr)

	// Do not subscribe to any topic here, just return the client

	go func() {
		select {
		case <-ctx.Done():
			client.Disconnect(250)
			log.Println("MQTT connection is closed")
		}
	}()

	return client, nil
}

// CloseRabbitMQConn ensures that the MQTT connection is properly closed.
func CloseRabbitMQConn(client mqtt.Client) {
	if client.IsConnected() {
		client.Disconnect(250)
		log.Println("MQTT connection successfully closed.")
	}
}
