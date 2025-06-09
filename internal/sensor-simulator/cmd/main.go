// cmd/sensor-sim/main.go
package main

import (
	"context"
	"flag"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model/entities"
	"log"
	"math"
	"time"

	sensorSimulator "github.com/LeonardoBeccarini/sdcc_project/internal/sensor-simulator"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
)

func main() {
	// define flags
	sensorID := flag.String("sensor-id", "sensor1", "unique sensor identifier")
	fieldID := flag.String("field-id", "field1", "unique field identifier")
	clientID := flag.String("client-id", "sensorPublisher1", "MQTT client ID")
	interval := flag.Duration("interval", 10*time.Second, "publish interval")
	lat := flag.Float64("lat", 12.37007, "latitude")
	lon := flag.Float64("lon", 41.51109, "longitude")
	maxDepth := flag.Int("depth", 30, "max soil depth")
	flowRate := flag.Float64("flow-rate", 10.0, "flow rate") //provvisorio
	flag.Parse()

	// inject flags into config
	cfg := &rabbitmq.RabbitMQConfig{
		Host:     "localhost",
		Port:     1883,
		User:     "guest",
		Password: "guest",
		ClientID: *clientID,
		Exchange: "sensor_data",
		Kind:     "topic",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := rabbitmq.NewRabbitMQConn(cfg, ctx)
	if err != nil {
		log.Fatal(err)
	}

	publisher := rabbitmq.NewPublisher(client, "sensor/data", cfg.Exchange)
	consumer := rabbitmq.NewConsumer(client, "event/stateChangeEvent", cfg.Exchange, nil)
	halfLife := 2 * time.Hour
	decayRate := math.Log(2) / halfLife.Seconds()
	generator := sensorSimulator.NewDataGenerator(decayRate)
	sensor := entities.Sensor{
		FieldId:   *fieldID,
		ID:        *sensorID,
		Longitude: *lon,
		Latitude:  *lat,
		MaxDepth:  *maxDepth,
		State:     entities.SensorState("off"),
		FlowRate:  *flowRate,
	}
	simulatedSensor := sensorSimulator.NewSensorSimulator(consumer, publisher, generator, &sensor)

	// pass sensorID through context or modify StartPublishing to accept it
	simulatedSensor.Start(ctx, *interval)
}
