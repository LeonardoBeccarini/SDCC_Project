package rabbitmq

import (
	"context"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"strings"
)

// IConsumer interface defines the ConsumeMessage method with dependencies T
type IConsumer[T any] interface {
	ConsumeMessage(ctx context.Context)
	SetHandler(handler func(queue string, message mqtt.Message) error)
}

// Consumer holds the client, topic, and exchange for subscribing to a topic
type Consumer struct {
	client  mqtt.Client
	handler func(queue string, message mqtt.Message) error
	topic   string
}

// NewConsumer creates a new Consumer instance using the shared MQTT client and topic
func NewConsumer(client mqtt.Client, topic string, handler func(queue string, message mqtt.Message) error) *Consumer {
	return &Consumer{
		client:  client,
		topic:   topic,
		handler: handler,
	}
}

func (c *Consumer) SetHandler(handler func(queue string, message mqtt.Message) error) {
	c.handler = handler
}

func qosFor(topic string) byte {
	t := strings.TrimSpace(topic)
	if strings.HasPrefix(t, "sensor/aggregated") ||
		strings.HasPrefix(t, "event/irrigationResult") ||
		strings.HasPrefix(t, "event/StateChange") {
		return 1
	}
	return 0
}

// ConsumeMessage subscribes to the topic and processes messages using the handler
// It blocks until the context is cancelled.
func (c *Consumer) ConsumeMessage(ctx context.Context) {
	token := c.client.Subscribe(
		c.topic,
		qosFor(c.topic),
		func(client mqtt.Client, message mqtt.Message) {
			if c.handler == nil {
				fmt.Printf("No handler set for topic %s\n", c.topic)
				return
			}
			err := c.handler(c.topic, message)
			if err != nil {
				fmt.Printf("Error handling message: %v\n", err)
			}
		},
	)

	if token.Wait() && token.Error() != nil {
		fmt.Printf("Error subscribing to topic %s: %v\n", c.topic, token.Error())
		return
	}

	fmt.Printf("Successfully subscribed to topic %s\n", c.topic)

	// Block here until context is done
	<-ctx.Done()

	// Unsubscribe when exiting to clean up
	unsubToken := c.client.Unsubscribe(c.topic)
	unsubToken.Wait()
}

// MultiConsumer -------------------------- [] ---------------------- [] ---------------------
type MultiConsumer struct {
	client   mqtt.Client
	topics   []string
	exchange string
	handler  func(queue string, message mqtt.Message) error
}

func NewMultiConsumer(client mqtt.Client, topics []string, handler func(queue string, message mqtt.Message) error) *MultiConsumer {
	return &MultiConsumer{
		client:  client,
		topics:  topics,
		handler: handler,
	}
}

func (m *MultiConsumer) SetHandler(handler func(queue string, message mqtt.Message) error) {
	m.handler = handler
}

func (m *MultiConsumer) ConsumeMessage(ctx context.Context) {
	for _, topic := range m.topics {
		topic := topic // shadow for closure safety
		token := m.client.Subscribe(
			topic,
			qosFor(topic),
			func(client mqtt.Client, msg mqtt.Message) {
				if m.handler == nil {
					fmt.Printf("No handler set for topic %s\n", topic)
					return
				}
				if err := m.handler(topic, msg); err != nil {
					fmt.Printf("Error handling message on %s: %v\n", topic, err)
				}
			},
		)
		token.Wait()
		if token.Error() != nil {
			fmt.Printf("Error subscribing to topic %s: %v\n", topic, token.Error())
		} else {
			fmt.Printf("Successfully subscribed to topic %s\n", topic)
		}
	}

	<-ctx.Done()

	// On context cancel: unsubscribe from all
	for _, topic := range m.topics {
		m.client.Unsubscribe(topic)
	}
}
