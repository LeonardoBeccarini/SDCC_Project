package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"

	persistencepkg "github.com/LeonardoBeccarini/sdcc_project/internal/services/persistence"
	rabbitmq "github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- MQTT (RabbitMQ/MQTT) ---
	host := env("RABBITMQ_HOST", env("MQTT_HOST", "localhost"))
	port := envInt("RABBITMQ_PORT", envInt("MQTT_PORT", 1883))
	user := env("RABBITMQ_USER", env("MQTT_USER", "mqtt_user"))
	pass := env("RABBITMQ_PASSWORD", env("MQTT_PASS", "mqtt_pwd"))
	exchange := env("RABBITMQ_EXCHANGE", env("MQTT_EXCHANGE", "sensor_data"))
	clientID := env("MQTT_CLIENT_ID", "persistence-service")
	topic := env("MQTT_TOPIC", env("AGGREGATED_SUB_TOPIC", "sensor/aggregated/#"))

	mqCfg := &rabbitmq.RabbitMQConfig{
		Host:     host,
		Port:     port,
		User:     user,
		Password: pass,
		ClientID: clientID,
		Exchange: exchange,
	}
	mqClient, err := rabbitmq.NewRabbitMQConn(mqCfg, ctx)
	if err != nil {
		log.Fatalf("mqtt connect failed: %v", err)
	}
	consumer := rabbitmq.NewConsumer(mqClient, topic, mqCfg.Exchange, nil)

	// --- InfluxDB ---
	influxURL := env("INFLUX_URL", "http://localhost:8086")
	influxToken := env("INFLUX_TOKEN", "")
	influxOrg := env("INFLUX_ORG", "org")
	influxBucket := env("INFLUX_BUCKET", "aggregated-data")
	measurement := env("MEASUREMENT", "soil_moisture")

	influxClient := influxdb2.NewClient(influxURL, influxToken)
	influxCfg := persistencepkg.InfluxConfig{
		InfluxURL:    influxURL,
		InfluxToken:  influxToken,
		InfluxOrg:    influxOrg,
		InfluxBucket: influxBucket,
		Measurement:  measurement,
	}

	// Service: consumer MQTT -> scrive su Influx e mantiene cache
	svc, err := persistencepkg.NewService(consumer, influxClient, influxCfg)
	if err != nil {
		log.Fatalf("persistence init failed: %v", err)
	}

	// --- HTTP mux ---
	// ATTENZIONE: /healthz è già registrato dentro NewHTTPMux(svc)
	mux := persistencepkg.NewHTTPMux(svc)

	// Aggiungi solo /readyz (se ti serve), NON /healthz per evitare conflitto
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ready": true})
	})

	httpPort := env("PORT", "8080")
	srv := &http.Server{
		Addr:              ":" + httpPort,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Avvia HTTP
	go func() {
		log.Printf("persistence HTTP listening on :%s", httpPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	// Avvia il consumo MQTT (e quindi scritture Influx)
	go svc.Start(ctx)

	<-ctx.Done()
	stop()

	// Graceful shutdown HTTP
	shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shCtx)
	log.Println("persistence: shutdown complete")
}
