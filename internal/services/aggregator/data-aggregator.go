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
	buffer              []model.SensorData
	mutex               sync.Mutex
	aggregationInterval time.Duration
}

func NewDataAggregatorService(consumer rabbitmq.IConsumer[model.SensorData], publisher rabbitmq.IPublisher, aggregationInterval time.Duration) *DataAggregatorService {
	return &DataAggregatorService{
		consumer:            consumer,
		publisher:           publisher,
		aggregationInterval: aggregationInterval,
		buffer:              make([]model.SensorData, 0),
	}
}

func (d *DataAggregatorService) messageHandler(queue string, message mqtt.Message) error {
	var sensorData model.SensorData
	if err := json.Unmarshal(message.Payload(), &sensorData); err != nil {
		log.Printf("Error unmarshalling sensor data: %v", err)
		return err
	}

	d.mutex.Lock()
	d.buffer = append(d.buffer, sensorData)
	d.mutex.Unlock()

	log.Printf("Buffered sensor data: %+v", sensorData)
	return nil
}

func (d *DataAggregatorService) Start(ctx context.Context) {
	//inject Handler function into the consumer
	d.consumer.SetHandler(d.messageHandler)
	// Start consuming the messages
	d.consumer.ConsumeMessage(nil)

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

	if len(d.buffer) == 0 {
		log.Println("No data to aggregate")
		return
	}

	// Calculate average values
	var (
		totalMoisture int
		//	totalTemperature float64
		//	totalWindSpeed   float64
		count = len(d.buffer)
	)

	for _, data := range d.buffer {
		totalMoisture += data.Moisture
		//	totalTemperature += data.Temperature
		//	totalWindSpeed += data.WindSpeed
	}

	// Create the aggregated data
	avgData := model.SensorData{
		SensorID:  "aggregated",
		Moisture:  totalMoisture / count,
		Timestamp: time.Now(),
	}

	// Marshal the aggregated data into JSON
	jsonData, err := json.Marshal(avgData)
	if err != nil {
		log.Printf("Error marshaling aggregated data: %v", err)
		return
	}

	// Publish the aggregated data
	if err := d.publisher.PublishMessage(string(jsonData)); err != nil {
		log.Printf("Error publishing aggregated data: %v", err)
	} else {
		log.Printf("Published aggregated data: %+v", avgData)
	}
	// Clear buffer after aggregation
	d.buffer = d.buffer[:0]
}
