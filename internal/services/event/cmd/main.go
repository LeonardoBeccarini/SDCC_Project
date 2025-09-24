package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"

	// aggiunte per dedup su Decision
	"crypto/sha256"
	"encoding/hex"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/dedup"

	"github.com/LeonardoBeccarini/sdcc_project/internal/services/event"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
)

func envStr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func main() {
	// === Config ===
	cfg := struct {
		Rabbit rabbitmq.RabbitMQConfig

		InfluxURL    string
		InfluxToken  string
		InfluxOrg    string
		InfluxBucket string

		Topics        []string
		BatchSize     int
		FlushInterval time.Duration

		HTTPPort       int
		ReadinessGrace time.Duration
	}{
		Rabbit: rabbitmq.RabbitMQConfig{
			Host:     envStr("RABBITMQ_HOST", "localhost"),
			Port:     envInt("RABBITMQ_PORT", 1883),
			User:     envStr("RABBITMQ_USER", "guest"),
			Password: envStr("RABBITMQ_PASSWORD", "guest"),
			ClientID: envStr("HOSTNAME", "event-service"),
			Kind:     envStr("RABBITMQ_EXCHANGE_KIND", "topic"),
		},

		InfluxURL:    envStr("INFLUX_URL", "http://localhost:8086"),
		InfluxToken:  os.Getenv("INFLUX_TOKEN"),
		InfluxOrg:    envStr("INFLUX_ORG", "msut"),
		InfluxBucket: envStr("INFLUX_BUCKET", "events"),

		Topics: func() []string {
			raw := envStr("EVENT_SUB_TOPICS", "event/irrigationDecision/#,event/StateChange/#")
			parts := strings.Split(raw, ",")
			out := make([]string, 0, len(parts))
			for _, p := range parts {
				if s := strings.TrimSpace(p); s != "" {
					out = append(out, s)
				}
			}
			return out
		}(),
		BatchSize:     envInt("WRITE_BATCH_SIZE", 10),
		FlushInterval: time.Duration(envInt("WRITE_FLUSH_INTERVAL_MS", 200)) * time.Millisecond,

		HTTPPort:       envInt("HTTP_PORT", 8080),
		ReadinessGrace: 5 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// === InfluxDB ===
	opts := influxdb2.DefaultOptions().
		SetBatchSize(uint(cfg.BatchSize)).
		SetFlushInterval(uint(cfg.FlushInterval.Milliseconds()))
	influx := influxdb2.NewClientWithOptions(cfg.InfluxURL, cfg.InfluxToken, opts)
	defer influx.Close()
	writeAPI := influx.WriteAPI(cfg.InfluxOrg, cfg.InfluxBucket)
	writer := event.NewWriter(writeAPI)

	// === MQTT ===
	mqttClient, err := rabbitmq.NewRabbitMQConn(&cfg.Rabbit, ctx)
	if err != nil {
		log.Fatalf("mqtt connection error: %v", err)
	}
	defer rabbitmq.CloseRabbitMQConn(mqttClient)

	// === HTTP ===
	mux := http.NewServeMux()
	mux.Handle("/healthz", event.NewHealthHandler(mqttClient, influx, writer))
	mux.Handle("/readyz", event.NewReadyHandler(mqttClient, influx, writer, 2*time.Second))

	// Rotta che IL GATEWAY sta chiamando oggi:
	// GET /events/irrigation/latest?limit=20[&minutes=1440]
	mux.Handle("/events/irrigation/latest", event.NewIrrigationLatestHandler(influx, cfg.InfluxOrg, cfg.InfluxBucket))

	hs := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.HTTPPort),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("event-svc: HTTP listening on :%d", cfg.HTTPPort)
		if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server error: %v", err)
		}
	}()

	// === Consumer ===
	h := event.NewMQTTHandler(func(evt event.CommonEvent) {
		p := event.EventToPoint(evt)
		writeAPI.WritePoint(p)
		writer.MarkIngest(evt.EventType)
	})

	// deduper condiviso SOLO per event/irrigationDecision/#
	d := dedup.New(10*time.Minute, 20000)

	for _, topic := range cfg.Topics {
		if strings.TrimSpace(topic) == "" {
			continue
		}
		log.Printf("event-svc: subscribing to %s", topic)

		// QoS per-topic: 1 solo per irrigationDecision, 0 per gli altri (es. StateChange)
		qos := byte(0)
		if strings.HasPrefix(topic, "event/irrigationDecision") {
			qos = 1
		}

		if token := mqttClient.Subscribe(topic, qos, func(_ mqtt.Client, m mqtt.Message) {
			// Dedup *solo* su event/irrigationDecision/# (QoS1 → possibili redelivery)
			if strings.HasPrefix(m.Topic(), "event/irrigationDecision/") {
				hh := sha256.Sum256(m.Payload())
				if !d.ShouldProcess(hex.EncodeToString(hh[:])) {
					return
				}
			}
			_ = h.Handle("", m) // logica invariata
		}); token.Wait() && token.Error() != nil {
			log.Fatalf("subscribe error on %s: %v", topic, token.Error())
		}
	}

	// === Wait for signal ===
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	log.Printf("event-svc: shutting down...")

	// graceful http
	shCtx, shCancel := context.WithTimeout(context.Background(), cfg.ReadinessGrace)
	defer shCancel()
	_ = hs.Shutdown(shCtx)

	// consenti flush
	time.Sleep(cfg.FlushInterval + 100*time.Millisecond)
}
