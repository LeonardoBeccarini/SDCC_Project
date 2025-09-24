package rabbitmq

import (
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"log"
)

type IPublisher interface {
	PublishMessage(message interface{}) error
	PublishMessageQos(qos byte, retained bool, message interface{}) error
	PublishTo(topic string, message interface{}) error
	PublishToQos(topic string, qos byte, retained bool, message interface{}) error
	Close()
}

type Publisher struct {
	client          mqtt.Client
	topic           string
	defaultQoS      byte
	defaultRetained bool
}

func NewPublisher(client mqtt.Client, topic string) *Publisher {
	return &Publisher{client: client, topic: topic, defaultQoS: 0, defaultRetained: false}
}

func NewPublisherWithQoS(client mqtt.Client, topic string, qos byte, retained bool) *Publisher {
	return &Publisher{client: client, topic: topic, defaultQoS: qos, defaultRetained: retained}
}

func (p *Publisher) PublishMessage(message interface{}) error {
	return p.PublishTo(p.topic, message)
}
func (p *Publisher) PublishMessageQos(qos byte, retained bool, message interface{}) error {
	return p.PublishToQos(p.topic, qos, retained, message)
}
func (p *Publisher) PublishTo(topic string, message interface{}) error {
	return p.PublishToQos(topic, p.defaultQoS, p.defaultRetained, message)
}
func (p *Publisher) PublishToQos(topic string, qos byte, retained bool, message interface{}) error {
	if token := p.client.Publish(topic, qos, retained, fmt.Sprintf("%v", message)); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish to %s: %v", topic, token.Error())
	}
	log.Printf("published %s (qos=%d retained=%v)", topic, qos, retained)
	return nil
}
func (p *Publisher) Close() {
	if p.client.IsConnected() {
		p.client.Disconnect(250)
		log.Println("MQTT client disconnected")
	}
}
