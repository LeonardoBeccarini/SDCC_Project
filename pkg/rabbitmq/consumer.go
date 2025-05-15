package rabbitmq

import (
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// IConsumer interface defines the ConsumeMessage method with dependencies T
type IConsumer[T any] interface {
	ConsumeMessage(msg interface{})
	SetHandler(handler func(queue string, message mqtt.Message) error)
}

// Consumer holds the client, topic, and exchange for subscribing to a topic
type Consumer struct {
	client   mqtt.Client
	handler  func(queue string, message mqtt.Message) error
	topic    string
	exchange string
}

// NewConsumer creates a new Consumer instance using the shared MQTT client and topic
func NewConsumer(client mqtt.Client, topic string, exchange string, handler func(queue string, message mqtt.Message) error) *Consumer {
	return &Consumer{
		client:   client,
		topic:    topic,
		exchange: exchange, // Set the exchange
		handler:  handler,
	}
}

func (c *Consumer) SetHandler(handler func(queue string, message mqtt.Message) error) {
	c.handler = handler
}

// ConsumeMessage subscribes to the topic and processes messages using the handler
func (c *Consumer) ConsumeMessage(msg interface{}) {
	token := c.client.Subscribe(
		c.topic,
		0,
		func(client mqtt.Client, message mqtt.Message) {
			// Call the handler when a message is received
			err := c.handler(c.topic, message)
			if err != nil {
				fmt.Printf("Error handling message: %v\n", err)
			}
		},
	)

	// Check if the subscription was successful
	if token.Wait() && token.Error() != nil {
		fmt.Printf("Error subscribing to topic %s: %v\n", c.topic, token.Error())
	} else {
		fmt.Printf("Successfully subscribed to topic %s\n", c.topic)
	}
}
