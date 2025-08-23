package rabbitmq

import (
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"log"
)

type IPublisher interface {
	PublishMessage(message interface{}) error
	PublishTo(topic string, message interface{}) error
	Close()
}

type Publisher struct {
	client   mqtt.Client
	topic    string
	exchange string
}

func NewPublisher(client mqtt.Client, topic string, exchange string) *Publisher {
	return &Publisher{client: client, topic: topic, exchange: exchange}
}

func (p *Publisher) PublishMessage(message interface{}) error {
	return p.PublishTo(p.topic, message)
}

func (p *Publisher) PublishTo(topic string, message interface{}) error {
	if token := p.client.Publish(topic, 0, false, fmt.Sprintf("%v", message)); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish message to %s: %v", topic, token.Error())
	}
	log.Printf("Message published to topic '%s'", topic)
	return nil
}

func (p *Publisher) Close() {
	if p.client.IsConnected() {
		p.client.Disconnect(250)
		log.Println("MQTT client disconnected")
	}
}
