package aggregator

import (
	"context"
	"encoding/json"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model/messages"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"log"
	"sync"
	"time"

	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
)

type DataAggregatorService struct {
	consumer            rabbitmq.IConsumer[messages.SensorData]
	publisher           rabbitmq.IPublisher
	buffer              map[string][]messages.SensorData // key is SensorID now
	mutex               sync.Mutex
	aggregationInterval time.Duration
}

func NewDataAggregatorService(consumer rabbitmq.IConsumer[messages.SensorData], publisher rabbitmq.IPublisher, aggregationInterval time.Duration) *DataAggregatorService {
	return &DataAggregatorService{
		consumer:            consumer,
		publisher:           publisher,
		aggregationInterval: aggregationInterval,
		buffer:              make(map[string][]messages.SensorData),
	}
}

func (d *DataAggregatorService) messageHandler(_ string, message mqtt.Message) error {
	var sensorData messages.SensorData
	if err := json.Unmarshal(message.Payload(), &sensorData); err != nil {
		log.Printf("Error unmarshalling sensor data: %v", err)
		return err
	}

	d.mutex.Lock()
	d.buffer[sensorData.SensorID] = append(d.buffer[sensorData.SensorID], sensorData)
	d.mutex.Unlock()

	log.Printf("Buffered sensor data: %+v", sensorData)
	return nil
}

func (d *DataAggregatorService) Start(ctx context.Context) {
	// Inject the handler
	d.consumer.SetHandler(d.messageHandler)

	// Run consumer in background, prima non era in una goroutine--> era bloccante e quindi il ticker non veniva mai raggiunto!!
	go d.consumer.ConsumeMessage(ctx)

	// Start aggregation ticker
	ticker := time.NewTicker(d.aggregationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.publisher.Close()
			return
		case <-ticker.C:
			log.Println("Running aggregation cycle")
			d.aggregateAndPublish()
		}
	}
}

func (d *DataAggregatorService) aggregateAndPublish() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	log.Println("Running aggregation cycle")

	for sensorID, readings := range d.buffer {
		if len(readings) == 0 {
			continue
		}
		sum := 0
		for _, r := range readings {
			sum += r.Moisture
		}
		avg := sum / len(readings)

		out := messages.SensorData{
			SensorID:   sensorID,
			FieldID:    readings[0].FieldID,
			Moisture:   avg,
			Aggregated: true,
			Timestamp:  time.Now(),
		}

		b, err := json.Marshal(out)
		if err != nil {
			log.Printf("marshal err %v", err)
			continue
		}
		if err := d.publisher.PublishMessage(string(b)); err != nil {
			log.Printf("publish err %v", err)
		} else {
			log.Printf("Published for %s: %+v", sensorID, out)
		}

		// reset buffer
		d.buffer[sensorID] = readings[:0]
	}
}
