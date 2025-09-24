package main

import (
	"context"
	"encoding/json"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
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
	host := mustGetenv("RABBITMQ_HOST")
	portStr := mustGetenv("RABBITMQ_PORT")
	user := mustGetenv("RABBITMQ_USER")
	pass := mustGetenv("RABBITMQ_PASSWORD")
	clientID := os.Getenv("MQTT_CLIENT_ID")
	if strings.TrimSpace(clientID) == "" {
		clientID = "sensor-simulator-" + strconv.FormatInt(time.Now().Unix(), 10)
	}
	intervalStr := os.Getenv("PUBLISH_INTERVAL")
	if intervalStr == "" {
		intervalStr = "15m"
	}
	configPath := os.Getenv("SENSORS_CONFIG_PATH")
	if configPath == "" {
		configPath = "/app/config/sensors-config.json"
	}
	sensorID := mustGetenv("SENSOR_ID")

	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil {
		log.Fatalf("Invalid RABBITMQ_PORT: %v", err)
	}

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		log.Fatalf("Invalid PUBLISH_INTERVAL: %v", err)
	}

	// Carica configurazione sensori
	configBytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		log.Fatalf("Failed to read config file %s: %v", configPath, err)
	}

	var config map[string][]model.Sensor
	if err := json.Unmarshal(configBytes, &config); err != nil {
		log.Fatalf("Failed to unmarshal sensors config: %v", err)
	}

	// Trova il sensore richiesto
	var sensor *entities.Sensor
	for _, arr := range config {
		for i := range arr {
			if arr[i].ID == sensorID {
				// model.Sensor è un alias di entities.Sensor, quindi il cast è sicuro
				s := entities.Sensor(arr[i])
				sensor = &s
				break
			}
		}
		if sensor != nil {
			break
		}
	}
	if sensor == nil {
		log.Fatalf("Sensor with id=%s not found in config", sensorID)
	}

	cfg := &rabbitmq.RabbitMQConfig{
		Host:     host,
		Port:     port,
		User:     user,
		Password: pass,
		ClientID: clientID,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := rabbitmq.NewRabbitMQConn(cfg, ctx)
	if err != nil {
		log.Fatalf("Failed to connect to MQTT broker: %v", err)
	}

	// Telemetria raw: sensor/data/{field}/{sensor}
	pubTopic := "sensor/data/" + sensor.FieldID + "/" + sensor.ID
	publisher := rabbitmq.NewPublisher(client, pubTopic)

	// Eventi di stato destinati al sensore
	topicTmpl := os.Getenv("STATE_CHANGE_TOPIC_TMPL")
	if strings.TrimSpace(topicTmpl) == "" {
		topicTmpl = "event/StateChange/{field}/{sensor}"
	}
	replacer := strings.NewReplacer("{field}", sensor.FieldID, "{sensor}", sensor.ID)
	stateTopic := replacer.Replace(topicTmpl)
	consumer := rabbitmq.NewConsumer(client, stateTopic, nil)

	// Generatore: seed UNA SOLA VOLTA da SoilGrids all'avvio; poi gestione interna
	gen := sensor_simulator.NewDataGenerator(0.001)
	// Seed iniziale da SoilGrids solo all'avvio
	gen.SeedFromSoilGrids(ctx, (*model.Sensor)(sensor))

	service := sensor_simulator.NewSensorSimulator(consumer, publisher, gen, sensor)
	log.Printf("Sensor Simulator [%s] started for field %s, pub=%s, sub=%s", sensor.ID, sensor.FieldID, pubTopic, stateTopic)
	service.Start(ctx, interval)
}
