package rabbitmq

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/eclipse/paho.mqtt.golang"
)

type RabbitMQConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	ClientID string
	Kind     string // Exchange type (topic, fanout, etc.)
}

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

	// Exponential backoff per le retry i caso di fail
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

	go func() {
		select {
		case <-ctx.Done():
			client.Disconnect(250)
			log.Println("MQTT connection is closed")
		}
	}()

	return client, nil
}

func CloseRabbitMQConn(client mqtt.Client) {
	if client.IsConnected() {
		client.Disconnect(250)
		log.Println("MQTT connection successfully closed.")
	}
}
