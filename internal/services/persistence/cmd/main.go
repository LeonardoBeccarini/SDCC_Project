package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"

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
	// === MQTT / RabbitMQ (plugin MQTT) ===
	host := env("MQTT_HOST", "localhost")
	port := mustInt("MQTT_PORT", 1883)
	user := env("MQTT_USER", "guest")
	pass := env("MQTT_PASS", "guest")
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
		ClientID: clientID,
	}

	client, err := rabbitmq.NewRabbitMQConn(rCfg, ctx)
	if err != nil {
		log.Fatalf("MQTT connect failed: %v", err)
	}

	consumer := rabbitmq.NewMultiConsumer(client, []string{topic}, nil)

	// === Influx ===
	influxCfg := persistence.InfluxConfig{
		InfluxURL:    env("INFLUX_URL", "http://localhost:8086"),
		InfluxToken:  env("INFLUX_TOKEN", ""),
		InfluxOrg:    env("INFLUX_ORG", "msut"),
		InfluxBucket: env("INFLUX_BUCKET", "events"),
		Measurement:  env("INFLUX_MEASUREMENT", "soil_moisture"),
	}

	influxClient := influxdb2.NewClient(influxCfg.InfluxURL, influxCfg.InfluxToken)
	defer influxClient.Close()

	svc, err := persistence.NewService(consumer, influxClient, influxCfg)
	if err != nil {
		log.Fatalf("persistence init failed: %v", err)
	}

	// === HTTP API ===
	mux := persistence.NewHTTPMux(svc)
	addr := ":" + strconv.Itoa(mustInt("PORT", 8080)) // EBS/Heroku-style
	httpSrv := &http.Server{Addr: addr, Handler: mux}

	// shutdown HTTP al termine
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shCtx)
	}()

	// avvia HTTP
	go func() {
		log.Printf("HTTP listening on %s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server error: %v", err)
		}
	}()

	log.Printf("Persistence running. sub=%s influx=%s/%s org=%s measurement=%s",
		topic, influxCfg.InfluxURL, influxCfg.InfluxBucket, influxCfg.InfluxOrg, influxCfg.Measurement)

	// avvia consumo MQTT e attendi stop
	svc.Start(ctx)
}
