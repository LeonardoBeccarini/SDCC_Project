package main

import (
	"context"
	"encoding/json"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model/entities"
	sensor_simulator "github.com/LeonardoBeccarini/sdcc_project/internal/sensor-simulator"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

func mustGetenv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("Missing required environment variable: %s", key)
	}
	return val
}

func main() {
	// Lettura variabili di ambiente
	host := mustGetenv("RABBITMQ_HOST")
	portStr := mustGetenv("RABBITMQ_PORT")
	user := mustGetenv("RABBITMQ_USER")
	pass := mustGetenv("RABBITMQ_PASSWORD")
	clientID := mustGetenv("CLIENT_ID")
	configPath := mustGetenv("CONFIG_PATH")
	intervalStr := mustGetenv("PUBLISH_INTERVAL")
	sensorID := mustGetenv("SENSOR_ID")

	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil {
		log.Fatalf("Invalid RABBITMQ_PORT: %v", err)
	}

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		log.Fatalf("Invalid PUBLISH_INTERVAL: %v", err)
	}

	// Lettura configurazione sensori dal ConfigMap
	configBytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		log.Fatalf("Failed to read config file %s: %v", configPath, err)
	}

	var config map[string][]entities.Sensor
	if err := json.Unmarshal(configBytes, &config); err != nil {
		log.Fatalf("Failed to unmarshal sensors config: %v", err)
	}

	// Cerca il sensore corrispondente a SENSOR_ID
	var sensor *entities.Sensor
	for _, sensors := range config {
		for _, s := range sensors {
			if s.ID == sensorID {
				sensor = &s
				break
			}
		}
	}
	if sensor == nil {
		log.Fatalf("Sensor with ID %s not found in config file", sensorID)
	}

	// Configurazione RabbitMQ
	cfg := &rabbitmq.RabbitMQConfig{
		Host:     host,
		Port:     port,
		User:     user,
		Password: pass,
		ClientID: clientID,
		Exchange: "sensor_data",
		Kind:     "topic",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := rabbitmq.NewRabbitMQConn(cfg, ctx)
	if err != nil {
		log.Fatalf("Failed to connect to MQTT broker: %v", err)
	}

	publisher := rabbitmq.NewPublisher(client, "field/"+sensor.FieldID+"/"+sensor.ID+"/data", cfg.Exchange)
	consumer := rabbitmq.NewConsumer(client, "event/stateChangeEvent", cfg.Exchange, nil)

	// Crea il generatore dati con un tasso di decadimento (es. 0.001 al secondo)
	gen := sensor_simulator.NewDataGenerator(0.001)

	// Avvia il simulatore
	service := sensor_simulator.NewSensorSimulator(consumer, publisher, gen, sensor)
	log.Printf("Sensor Simulator [%s] started for field %s", sensor.ID, sensor.FieldID)
	service.Start(ctx, interval)
}
