// sensor_simulator.go
package sensor_simulator

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
	"log"
	"sync"
	"time"

	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type SensorSimulator struct {
	mu        sync.Mutex
	sensor    *model.Sensor // only one sensor here
	timer     *time.Timer   // single timer
	generator *DataGenerator
	publisher rabbitmq.IPublisher
	consumer  rabbitmq.IConsumer[mqtt.Message]
}

// NewSensorSimulator takes exactly one Sensor entity
func NewSensorSimulator(
	consumer rabbitmq.IConsumer[mqtt.Message],
	publisher rabbitmq.IPublisher,
	gen *DataGenerator,
	sensor *model.Sensor,
) *SensorSimulator {
	return &SensorSimulator{
		sensor:    sensor,
		generator: gen,
		publisher: publisher,
		consumer:  consumer,
	}
}

// Start wires up the consumer and begins both consuming state‐changes
// and publishing SensorData every interval.
func (s *SensorSimulator) Start(
	ctx context.Context,
	interval time.Duration,
) {
	// 1) State‐change consumption
	s.consumer.SetHandler(s.handleMessage)
	go s.consumer.ConsumeMessage(ctx)

	// 2) Publish loop
	for {
		select {
		case <-ctx.Done():
			s.publisher.Close()
			return
		case <-time.After(interval):
			sd, err := s.generator.Next(
				s.sensor)
			if err != nil {
				log.Printf("data gen error: %v", err)
				continue
			}
			payload, _ := json.Marshal(sd)
			if err := s.publisher.PublishMessage(string(payload)); err != nil {
				log.Printf("publish error: %v", err)
			}
		}
	}
}

// handleMessage expects StateChangeEvent for *this* sensor
func (s *SensorSimulator) handleMessage(queue string, msg mqtt.Message) error {
	var evt model.StateChangeEvent
	if err := json.Unmarshal(msg.Payload(), &evt); err != nil {
		return fmt.Errorf("invalid StateChangeEvent: %w", err)
	}
	if evt.SensorID != s.sensor.ID {
		// ignore events for other sensors
		return nil
	}
	s.applyTimedState(evt)
	return nil
}

func (s *SensorSimulator) applyTimedState(evt model.StateChangeEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// cancel prior timer if running
	if s.timer != nil {
		s.timer.Stop()
	}

	// remember previous state
	prev := s.sensor.State

	// apply new state
	s.sensor.State = evt.NewState
	fmt.Printf("Sensor %s → %s for %s\n", s.sensor.ID, evt.NewState, evt.Duration)

	// Se l'irrigazione va in ON, riflette subito l'acqua applicata nella moisture
	if evt.NewState == model.StateOn && s.generator != nil {
		s.generator.ApplyIrrigation(evt.Duration)
	}

	// schedule a revert
	s.timer = time.AfterFunc(evt.Duration, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.sensor.State = prev
		fmt.Printf("Sensor %s ↺ %s\n", s.sensor.ID, prev)
		s.timer = nil
		// TODO chi è responsabile di fare una query ai sensori per verificarne lo stato?
	})
}
