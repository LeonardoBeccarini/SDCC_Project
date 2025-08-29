package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/LeonardoBeccarini/sdcc_project/internal/services/persistence"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("invalid int for %s=%q, using %d", key, v, def)
		return def
	}
	return n
}

func main() {
	// --- MQTT / RabbitMQ (plugin MQTT) ---
	host := env("MQTT_HOST", "localhost")
	port := mustInt("MQTT_PORT", 1883)
	user := env("MQTT_USER", "guest")
	pass := env("MQTT_PASS", "guest")
	exchange := env("MQTT_EXCHANGE", "")
	clientID := env("MQTT_CLIENT_ID", "persistence-"+strconv.FormatInt(time.Now().Unix(), 10))

	// Topic pubblicati dall’aggregator: sensor/aggregated/{field}/{sensor}
	topic := env("MQTT_TOPIC", "sensor/aggregated/#")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	rCfg := &rabbitmq.RabbitMQConfig{
		Host:     host,
		Port:     port,
		User:     user,
		Password: pass,
		Exchange: exchange,
		ClientID: clientID,
	}

	client, err := rabbitmq.NewRabbitMQConn(rCfg, ctx)
	if err != nil {
		log.Fatalf("MQTT connect failed: %v", err)
	}

	consumer := rabbitmq.NewMultiConsumer(client, []string{topic}, exchange, nil)

	// --- Influx config ---
	influxCfg := persistence.InfluxConfig{
		InfluxURL:       env("INFLUX_URL", "http://localhost:8086"),
		InfluxToken:     env("INFLUX_TOKEN", ""),
		InfluxOrg:       env("INFLUX_ORG", "msut"),
		InfluxBucket:    env("INFLUX_BUCKET", "events"),
		MeasurementMode: strings.ToLower(env("MEASUREMENT_MODE", "per-sensor")), // "per-sensor" | "single"
		MeasurementName: env("MEASUREMENT_NAME", "soil_moisture"),               // usato se mode == "single"
	}

	svc, err := persistence.NewService(consumer, influxCfg)
	if err != nil {
		log.Fatalf("persistence init failed: %v", err)
	}

	log.Printf("Persistence running. sub=%s influx=%s/%s org=%s mode=%s name=%s",
		topic, influxCfg.InfluxURL, influxCfg.InfluxBucket, influxCfg.InfluxOrg,
		influxCfg.MeasurementMode, influxCfg.MeasurementName)

	svc.Start(ctx)
}
