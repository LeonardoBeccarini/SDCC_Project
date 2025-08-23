package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	controller "github.com/LeonardoBeccarini/sdcc_project/internal/services/irrigation-controller"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
)

func env(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}
func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// MQTT
	host := env("RABBITMQ_HOST", "localhost")
	port := envInt("RABBITMQ_PORT", 1883)
	user := env("RABBITMQ_USER", "guest")
	pass := env("RABBITMQ_PASSWORD", "guest")
	exchange := env("RABBITMQ_EXCHANGE", "sensor_data")
	clientID := fmt.Sprintf("IrrigationController-%s", env("HOSTNAME", "local"))

	cfg := &rabbitmq.RabbitMQConfig{Host: host, Port: port, User: user, Password: pass, ClientID: clientID, Exchange: exchange}
	mqClient, err := rabbitmq.NewRabbitMQConn(cfg, ctx)
	if err != nil {
		log.Fatalf("MQTT connect failed: %v", err)
	}

	aggregatedSub := env("AGGREGATED_SUB_TOPIC", "sensor/aggregated/#")
	decisionTopicTmpl := env("IRRIGATION_DECISION_TOPIC_TMPL", "event/irrigationDecision/{field}/{sensor}")

	consumer := rabbitmq.NewConsumer(mqClient, aggregatedSub, cfg.Exchange, nil)
	decisionPublisher := rabbitmq.NewPublisher(mqClient, "event/irrigationDecision", cfg.Exchange)

	// OpenWeather client
	owmKey := env("OWM_API_KEY", "changeme")
	wc := controller.NewOWMClient(owmKey)

	// Device routing: field -> deviceService endpoint
	mapStr := env("DEVICE_GRPC_ADDR_MAP", "field1=device-node1:50051,field2=device-node2:50051")
	router, err := controller.NewDeviceRouter(ctx, mapStr)
	if err != nil {
		log.Fatalf("device router init: %v", err)
	}
	defer router.Close()

	// Config file paths
	policyPath := env("FIELD_POLICY_PATH", "/app/config/field-policy.json")
	sensorsPath := env("SENSORS_CONFIG_PATH", "/app/config/sensors-config.json")

	ctrl, err := controller.NewController(
		consumer,
		decisionPublisher,
		router,
		wc,
		policyPath,
		sensorsPath,
		decisionTopicTmpl,
	)
	if err != nil {
		log.Fatalf("controller init: %v", err)
	}

	log.Printf("IrrigationController running. sub=%s decisions=%s routes=%s", aggregatedSub, decisionTopicTmpl, mapStr)
	ctrl.Start(ctx)
}
