package sensor_simulator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/dedup"

	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type SensorSimulator struct {
	mu        sync.Mutex
	sensor    *model.Sensor
	timer     *time.Timer // single timer
	generator *DataGenerator
	publisher rabbitmq.IPublisher
	consumer  rabbitmq.IConsumer[mqtt.Message]
	deduper   *dedup.Deduper
}

func NewSensorSimulator(consumer rabbitmq.IConsumer[mqtt.Message], publisher rabbitmq.IPublisher,
	gen *DataGenerator, sensor *model.Sensor) *SensorSimulator {
	return &SensorSimulator{
		sensor:    sensor,
		generator: gen,
		publisher: publisher,
		consumer:  consumer,
		deduper:   dedup.New(2*time.Minute, 10000), // TTL e cap
	}
}

// Start avvia il simulatore,avvia ricezione dei cambiamenti di stato e pubblicazione dei dati a intervalli regolari.
func (s *SensorSimulator) Start(
	ctx context.Context,
	interval time.Duration,
) {
	// State‐change
	s.consumer.SetHandler(s.handleMessage)
	go s.consumer.ConsumeMessage(ctx)

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
			//debug
			log.Printf("sensor: pub raw field=%s sensor=%s moisture=%d%%",
				sd.FieldID, sd.SensorID, sd.Moisture)
			payload, _ := json.Marshal(sd)
			if err := s.publisher.PublishMessage(string(payload)); err != nil {
				log.Printf("publish error: %v", err)
			}
		}
	}
}

func (s *SensorSimulator) handleMessage(_ string, msg mqtt.Message) error {
	// Dedup a payload: redelivery QoS1 ha lo stesso payload → stesso hash
	h := sha256.Sum256(msg.Payload())
	if s.deduper != nil && !s.deduper.ShouldProcess(hex.EncodeToString(h[:])) {
		return nil // duplicato → ignora
	}

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

	if s.timer != nil {
		s.timer.Stop()
	}

	prev := s.sensor.State

	// apply new state
	s.sensor.State = evt.NewState
	fmt.Printf("Sensor %s → %s for %s\n", s.sensor.ID, evt.NewState, evt.Duration)

	// Se l'irrigazione va in ON, riflette subito l'acqua applicata nella moisture
	if evt.NewState == model.StateOn && s.generator != nil {
		s.generator.ApplyIrrigation(evt.Duration)
	}

	// schedule a revert
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}

	if evt.Duration > 0 {
		s.timer = time.AfterFunc(evt.Duration, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.sensor.State = prev
			fmt.Printf("Sensor %s ↺ %s\n", s.sensor.ID, prev)
			s.timer = nil
		})
	}
}
