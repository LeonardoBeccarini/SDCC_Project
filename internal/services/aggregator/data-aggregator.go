package aggregator

import (
	"context"
	"encoding/json"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"log"
	"sync"
	"time"

	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
)

type DataAggregatorService struct {
	consumer            rabbitmq.IConsumer[model.SensorData]
	publisher           rabbitmq.IPublisher
	buffer              map[string][]model.SensorData
	mutex               sync.Mutex
	aggregationInterval time.Duration
}

func NewDataAggregatorService(consumer rabbitmq.IConsumer[model.SensorData], publisher rabbitmq.IPublisher, aggregationInterval time.Duration) *DataAggregatorService {
	return &DataAggregatorService{
		consumer:            consumer,
		publisher:           publisher,
		aggregationInterval: aggregationInterval,
		buffer:              make(map[string][]model.SensorData),
	}
}

func (d *DataAggregatorService) messageHandler(_ string, message mqtt.Message) error {
	var sensorData model.SensorData
	if err := json.Unmarshal(message.Payload(), &sensorData); err != nil {
		log.Printf("Error unmarshalling sensor data: %v", err)
		return err
	}

	d.mutex.Lock()
	d.buffer[sensorData.FieldID] = append(d.buffer[sensorData.FieldID], sensorData)
	d.mutex.Unlock()

	log.Printf("Buffered sensor data: %+v", sensorData)
	return nil
}

func (d *DataAggregatorService) Start(ctx context.Context) {
	//inject Handler function into the consumer
	d.consumer.SetHandler(d.messageHandler)
	// Start consuming the messages
	d.consumer.ConsumeMessage(ctx)

	// Periodic aggregation
	ticker := time.NewTicker(d.aggregationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.publisher.Close()
			return
		case <-ticker.C:
			d.aggregateAndPublish()
		}
	}
}

func (d *DataAggregatorService) aggregateAndPublish() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	for fieldID, readings := range d.buffer {
		if len(readings) == 0 {
			continue
		}
		sum := 0
		for _, r := range readings {
			sum += r.Moisture
		}
		avg := sum / len(readings)
		out := model.SensorData{
			SensorID:  "agg-" + fieldID,
			FieldID:   fieldID,
			Moisture:  avg,
			Timestamp: time.Now(),
		}
		b, err := json.Marshal(out)
		if err != nil {
			log.Printf("marshal err %v", err)
			continue
		}
		if err := d.publisher.PublishMessage(string(b)); err != nil {
			log.Printf("publish err %v", err)
		} else {
			log.Printf("Published for %s: %+v", fieldID, out)
		}
		// reset
		d.buffer[fieldID] = readings[:0]
	}
}
