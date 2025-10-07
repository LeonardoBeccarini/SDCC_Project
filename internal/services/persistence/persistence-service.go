package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	//per dedup QoS1
	"crypto/sha256"
	"encoding/hex"

	"github.com/LeonardoBeccarini/sdcc_project/pkg/dedup"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"

	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
)

type InfluxConfig struct {
	InfluxURL    string
	InfluxToken  string
	InfluxOrg    string
	InfluxBucket string
	Measurement  string // default: "soil_moisture"
}

type Service struct {
	consumer    rabbitmq.IConsumer[model.SensorData]
	writeAPI    api.WriteAPIBlocking
	queryAPI    api.QueryAPI
	cfg         InfluxConfig
	measurement string

	mu     sync.RWMutex
	latest map[string]model.SensorData // cache: key = field_id/sensor_id

	//deduper per scartare redelivery QoS1
	deduper *dedup.Deduper
}

// Costruttore: passa il consumer MQTT e il client Influx già creato nel main.
func NewService(consumer rabbitmq.IConsumer[model.SensorData], client influxdb2.Client, cfg InfluxConfig) (*Service, error) {
	if cfg.Measurement == "" {
		cfg.Measurement = "soil_moisture"
	}
	w := client.WriteAPIBlocking(cfg.InfluxOrg, cfg.InfluxBucket)
	q := client.QueryAPI(cfg.InfluxOrg)
	s := &Service{
		consumer:    consumer,
		writeAPI:    w,
		queryAPI:    q,
		cfg:         cfg,
		measurement: cfg.Measurement,
		latest:      make(map[string]model.SensorData),
		// ⬇️ init deduper (TTL 10m, cap 20k)
		deduper: dedup.New(10*time.Minute, 20000),
	}
	// Il service gestisce la deserializzazione e la scrittura su Influx
	consumer.SetHandler(s.handleMessage)
	return s, nil
}

func keyOf(fieldID, sensorID string) string { return fieldID + "/" + sensorID }

// Handler dei messaggi MQTT (aggregati) -> scrive su Influx + aggiorna cache
func (s *Service) handleMessage(_ string, msg mqtt.Message) error {
	// DEDUP PRIMA DI UNMARSHAL: scarta redelivery QoS1 identiche
	h := sha256.Sum256(msg.Payload())
	if s.deduper != nil && !s.deduper.ShouldProcess(hex.EncodeToString(h[:])) {
		return nil
	}

	var data model.SensorData
	if err := json.Unmarshal(msg.Payload(), &data); err != nil {
		log.Printf("persistence: bad payload: %v", err)
		return nil // non nack
	}
	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now().UTC()
	}

	// write Influx
	p := influxdb2.NewPoint(
		s.measurement,
		map[string]string{
			"field_id":   safeTag(data.FieldID),
			"sensor_id":  safeTag(data.SensorID),
			"aggregated": boolToStr(data.Aggregated),
		},
		map[string]interface{}{
			"moisture": int64(data.Moisture),
		},
		data.Timestamp,
	)
	if err := s.writeAPI.WritePoint(context.Background(), p); err != nil {
		log.Printf("persistence: write influx: %v", err)
	}

	// cache
	s.mu.Lock()
	s.latest[keyOf(data.FieldID, data.SensorID)] = data
	s.mu.Unlock()
	return nil
}

// Start: avvia il consumo dei messaggi finché il contesto non viene cancellato
func (s *Service) Start(ctx context.Context) {
	go s.consumer.ConsumeMessage(ctx)
	<-ctx.Done()
}

// Query: ultimi valori (aggregated=true) da Influx per ciascun sensore
func (s *Service) QueryLatestFromInflux(ctx context.Context, minutes int) ([]model.SensorData, error) {
	if minutes <= 0 {
		minutes = 60 * 24
	}
	flux := fmt.Sprintf(`from(bucket: "%s")
  |> range(start: -%dm)
  |> filter(fn: (r) => r._measurement == "%s")
  |> filter(fn: (r) => r._field == "moisture")
  |> filter(fn: (r) => r.aggregated == "true")
  |> group(columns: ["field_id","sensor_id"])
  |> last()
  |> keep(columns: ["_time","_value","field_id","sensor_id"])`,
		s.cfg.InfluxBucket, minutes, s.measurement)

	res, err := s.queryAPI.Query(ctx, flux)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := res.Close(); cerr != nil {
		}
	}()

	out := make([]model.SensorData, 0, 64)
	for res.Next() {
		rec := res.Record()
		var fid, sid string
		if v, ok := rec.ValueByKey("field_id").(string); ok {
			fid = v
		}
		if v, ok := rec.ValueByKey("sensor_id").(string); ok {
			sid = v
		}
		var moisture int
		switch v := rec.Value().(type) {
		case float64:
			moisture = int(math.Round(v))
		case int64:
			moisture = int(v)
		default:
			moisture = 0
		}
		out = append(out, model.SensorData{
			FieldID:    fid,
			SensorID:   sid,
			Moisture:   moisture,
			Aggregated: true,
			Timestamp:  rec.Time().UTC(),
		})
	}
	if res.Err() != nil {
		return nil, res.Err()
	}
	return out, nil
}

// Cache read (fallback)
func (s *Service) LatestCache() []model.SensorData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.SensorData, 0, len(s.latest))
	for _, v := range s.latest {
		out = append(out, v)
	}
	return out
}

func boolToStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func safeTag(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '_', r == ':', r == '-':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}
