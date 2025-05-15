package rabbitmq

import (
	"fmt"
	"github.com/eclipse/paho.mqtt.golang"
	"log"
)

// IPublisher interface defines the method to publish a message
type IPublisher interface {
	PublishMessage(message interface{}) error
	Close()
}

// Publisher holds the client, topic, and exchange for publishing messages
type Publisher struct {
	client   mqtt.Client
	topic    string
	exchange string
}

// NewPublisher creates a new Publisher instance using the shared MQTT client and topic
func NewPublisher(client mqtt.Client, topic string, exchange string) *Publisher {
	// TODO qui prima c'era un controllo per verificare che il client passato no fosse null, servirà?
	return &Publisher{
		client:   client,
		topic:    topic,
		exchange: exchange, // Set the exchange
	}
}

// PublishMessage publishes a message to the given MQTT topic
func (p *Publisher) PublishMessage(message interface{}) error {
	messageStr, ok := message.(string)
	if !ok {
		return fmt.Errorf("invalid message format, expected string")
	}

	// Publish the message to the MQTT topic (which is tied to an exchange)
	token := p.client.Publish(p.topic, 0, false, messageStr) // Set QoS to 0 (At most once)
	token.Wait()

	if token.Error() != nil {
		return fmt.Errorf("failed to publish message: %v", token.Error())
	}

	log.Printf("Message '%s' published to topic '%s'", messageStr, p.topic)
	return nil
}

// Close gracefully closes the MQTT connection for the publisher
func (p *Publisher) Close() {
	if p.client.IsConnected() {
		p.client.Disconnect(250)
		log.Println("MQTT client disconnected")
	}
}
