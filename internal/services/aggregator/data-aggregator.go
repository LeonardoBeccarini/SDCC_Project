package aggregator

import (
	"context"
	"encoding/json"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
	"log"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
)

type DataAggregatorService struct {
	consumer  rabbitmq.IConsumer[model.SensorData]
	publisher rabbitmq.IPublisher
	window    time.Duration

	mu     sync.Mutex
	buffer map[string][]model.SensorData // key = fieldID|sensorID
}

func NewDataAggregatorService(consumer rabbitmq.IConsumer[model.SensorData], publisher rabbitmq.IPublisher, window time.Duration) *DataAggregatorService {
	if window <= 0 {
		window = 15 * time.Minute
	}
	return &DataAggregatorService{
		consumer:  consumer,
		publisher: publisher,
		window:    window,
		buffer:    make(map[string][]model.SensorData),
	}
}

func (d *DataAggregatorService) Start(ctx context.Context) {
	d.consumer.SetHandler(func(_ string, msg mqtt.Message) error {
		var s model.SensorData
		if err := json.Unmarshal(msg.Payload(), &s); err != nil {
			log.Printf("aggregate: bad payload: %v", err)
			return nil
		}
		if s.Aggregated {
			return nil
		}
		key := s.FieldID + "|" + s.SensorID
		d.mu.Lock()
		d.buffer[key] = append(d.buffer[key], s)
		d.mu.Unlock()
		return nil
	})

	ticker := time.NewTicker(d.window)
	defer ticker.Stop()

	go d.consumer.ConsumeMessage(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.flush()
		}
	}
}

func (d *DataAggregatorService) flush() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for key, readings := range d.buffer {
		if len(readings) == 0 {
			continue
		}

		sum := 0
		for _, r := range readings {
			sum += r.Moisture
		}
		avg := int(float64(sum)/float64(len(readings)) + 0.5)

		// split key
		i := -1
		for j := 0; j < len(key); j++ {
			if key[j] == '|' {
				i = j
				break
			}
		}
		fieldID, sensorID := "", ""
		if i >= 0 {
			fieldID, sensorID = key[:i], key[i+1:]
		} else {
			fieldID, sensorID = readings[0].FieldID, readings[0].SensorID
		}

		out := model.SensorData{
			FieldID:    fieldID,
			SensorID:   sensorID,
			Moisture:   avg,
			Aggregated: true,
			Timestamp:  time.Now().UTC(),
		}
		b, _ := json.Marshal(out)

		topic := "sensor/aggregated/" + fieldID + "/" + sensorID
		if err := d.publisher.PublishTo(topic, string(b)); err != nil {
			log.Printf("aggregate: publish err %v", err)
		} else {
			log.Printf("aggregate: published %s %s -> %d%%", fieldID, sensorID, avg)
		}
		// clear
		d.buffer[key] = readings[:0]
	}
}
