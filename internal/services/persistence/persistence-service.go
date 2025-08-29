package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"

	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
)

// Configurazione Influx
type InfluxConfig struct {
	InfluxURL       string
	InfluxToken     string
	InfluxOrg       string
	InfluxBucket    string
	MeasurementMode string // "per-sensor" | "single"
	MeasurementName string // se "single", es. "soil_moisture"
}

type Service struct {
	consumer        rabbitmq.IConsumer[model.SensorData]
	writeAPI        api.WriteAPIBlocking
	measurementMode string
	measurementName string
}

func NewService(consumer rabbitmq.IConsumer[model.SensorData], cfg InfluxConfig) (*Service, error) {
	if cfg.InfluxURL == "" || cfg.InfluxToken == "" || cfg.InfluxOrg == "" || cfg.InfluxBucket == "" {
		return nil, fmt.Errorf("influx config incomplete")
	}

	client := influxdb2.NewClient(cfg.InfluxURL, cfg.InfluxToken)
	writeAPI := client.WriteAPIBlocking(cfg.InfluxOrg, cfg.InfluxBucket)

	return &Service{
		consumer:        consumer,
		writeAPI:        writeAPI,
		measurementMode: cfg.MeasurementMode,
		measurementName: cfg.MeasurementName,
	}, nil
}

func (s *Service) Start(ctx context.Context) {
	// Handler invocato dal consumer per ogni messaggio MQTT
	s.consumer.SetHandler(func(topic string, msg mqtt.Message) error {
		var m model.SensorData
		if err := json.Unmarshal(msg.Payload(), &m); err != nil {
			log.Printf("persistence: invalid JSON on %s: %v", topic, err)
			return nil // non bloccare lo stream
		}

		// Scegli il measurement name
		measurement := s.measurementName
		if s.measurementMode == "per-sensor" {
			if measurement == "" {
				measurement = "measurement"
			}
			measurement = measurement + "_" + m.SensorID // es. soil_moisture_<sensor>
		}
		measurement = sanitizeMeasurement(measurement)

		// Tag & Fields
		t := m.Timestamp
		if t.IsZero() {
			t = time.Now()
		}

		tags := map[string]string{
			"field_id":  m.FieldID,
			"sensor_id": m.SensorID,
		}
		fields := map[string]interface{}{
			"moisture":   m.Moisture,
			"aggregated": m.Aggregated,
		}

		// ✅ CORRETTO: usa NewPoint (niente struct literal con Time/Name/Tags/Fields)
		point := influxdb2.NewPoint(measurement, tags, fields, t)

		if err := s.writeAPI.WritePoint(ctx, point); err != nil {
			log.Printf("persistence: write error: %v", err)
			return err
		}
		log.Printf("persistence: wrote %s field=%s sensor=%s moisture=%d",
			measurement, m.FieldID, m.SensorID, m.Moisture)
		return nil
	})

	// Avvia il loop di consumo (blocca finché il contesto non chiude)
	s.consumer.ConsumeMessage(ctx)
}

func sanitizeMeasurement(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '_', r == ':', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
