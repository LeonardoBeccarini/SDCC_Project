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
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// MQTT
	host := env("RABBITMQ_HOST", "localhost")
	port := envInt("RABBITMQ_PORT", 1883)
	user := env("RABBITMQ_USER", "guest")
	pass := env("RABBITMQ_PASSWORD", "guest")
	clientID := fmt.Sprintf("IrrigationController-%s", env("HOSTNAME", "local"))

	cfg := &rabbitmq.RabbitMQConfig{Host: host, Port: port, User: user, Password: pass, ClientID: clientID, Kind: "topic"}
	mqClient, err := rabbitmq.NewRabbitMQConn(cfg, ctx)
	if err != nil {
		log.Fatalf("MQTT connect failed: %v", err)
	}

	aggregatedSub := env("AGGREGATED_SUB_TOPIC", "sensor/aggregated/#")
	resultSub := env("IRRIGATION_RESULT_SUB", "event/irrigationResult/#")

	consumer := rabbitmq.NewConsumer(mqClient, aggregatedSub, nil)

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
		router,
		wc,
		policyPath,
		sensorsPath,
	)
	if err != nil {
		log.Fatalf("controller init: %v", err)
	}

	// Consumer dei Result (qos=1 gestito a livello consumer)
	resConsumer := rabbitmq.NewConsumer(mqClient, resultSub, nil)
	ctrl.AttachResultConsumer(resConsumer)

	log.Printf("IrrigationController running. sub=%s routes=%s resultSub=%s", aggregatedSub, mapStr, resultSub)
	ctrl.Start(ctx)

	// graceful shutdown
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	<-sigc
	cancel()
}
