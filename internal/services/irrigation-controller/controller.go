package irrigation_controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	pb "github.com/LeonardoBeccarini/sdcc_project/grpc/gen/go/irrigation"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// DeviceRouter espone un client gRPC per ogni field (field -> DeviceService)
type DeviceRouter interface {
	Get(field string) (pb.DeviceServiceClient, bool)
	Close()
}

// WeatherClient restituisce ET0 e pioggia giornaliera (mm) per lat/lon/data
type WeatherClient interface {
	GetDailyET0AndRain(ctx context.Context, lat, lon float64, day time.Time) (etoMM, rainMM float64, err error)
}

type Controller struct {
	consumer          rabbitmq.IConsumer[model.SensorData]
	publisher         rabbitmq.IPublisher
	router            DeviceRouter
	wclient           WeatherClient
	sensors           map[string]map[string]model.Sensor // field -> sensorID -> Sensor
	moistureThreshold float64                            // %
	decisionTopicTmpl string
	mu                sync.RWMutex
}

// NewController crea il controller unico con router gRPC per i device.
// fieldPolicyPath è ignorato (non esiste un file policy nello zip) ma mantenuto per compatibilità con main.go.
func NewController(
	c rabbitmq.IConsumer[model.SensorData],
	p rabbitmq.IPublisher,
	router DeviceRouter,
	wc WeatherClient,
	_ string, // fieldPolicyPath (unused)
	sensorsPath string,
	decisionTopicTmpl string,
) (*Controller, error) {

	if router == nil {
		return nil, errors.New("device router is nil")
	}
	if wc == nil {
		return nil, errors.New("weather client is nil")
	}

	sensors, err := loadSensors(sensorsPath)
	if err != nil {
		return nil, fmt.Errorf("load sensors: %w", err)
	}

	// soglia da env (default 35%)
	th := 35.0
	if v := strings.TrimSpace(os.Getenv("MOISTURE_THRESHOLD_PCT")); v != "" {
		if f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64); err == nil && f > 0 {
			th = f
		}
	}

	if strings.TrimSpace(decisionTopicTmpl) == "" {
		decisionTopicTmpl = "event/irrigationDecision/{field}/{sensor}"
	}

	ctrl := &Controller{
		consumer:          c,
		publisher:         p,
		router:            router,
		wclient:           wc,
		sensors:           sensors,
		moistureThreshold: th,
		decisionTopicTmpl: decisionTopicTmpl,
	}
	c.SetHandler(ctrl.handleAggregated)
	return ctrl, nil
}

func (c *Controller) Start(ctx context.Context) {
	go c.consumer.ConsumeMessage(ctx)
	<-ctx.Done()
}

// handler dei messaggi aggregati: sensor/aggregated/{field}/{sensor}
func (c *Controller) handleAggregated(_ string, msg mqtt.Message) error {
	var s model.SensorData
	if err := json.Unmarshal(msg.Payload(), &s); err != nil {
		log.Printf("controller: bad payload: %v", err)
		return nil
	}
	if !s.Aggregated {
		return nil
	}

	fieldID, sensorID := s.FieldID, s.SensorID

	// recupera il sensore
	sensor, ok := c.lookupSensor(fieldID, sensorID)
	if !ok {
		log.Printf("controller: unknown sensor %s/%s", fieldID, sensorID)
		return nil
	}

	// client gRPC corretto per field
	device, ok := c.router.Get(fieldID)
	if !ok {
		log.Printf("controller: no device client for field=%s", fieldID)
		return nil
	}

	// meteo (ET0 & rain)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	eto, rain, err := c.wclient.GetDailyET0AndRain(ctx, sensorLatitude(sensor), sensorLongitude(sensor), time.Now().UTC())
	if err != nil {
		log.Printf("controller: weather error: %v (using defaults)", err)
		eto, rain = 4.0, 0.0
	}

	// decisione semplice: se moisture (%) sotto soglia → irriga
	doseMM := 0.0
	if float64(s.Moisture) < c.moistureThreshold {
		base := 5.0 // mm di base
		etoAdj := math.Max(0, eto-rain)
		doseMM = base + 0.5*etoAdj
	}

	// traduzione dose → minuti con flow/area
	mmPerMin := flowMMPerMinute(sensor)
	durationMin := 0
	if doseMM > 0 && mmPerMin > 0 {
		durationMin = int(math.Round(doseMM / mmPerMin))
		if durationMin <= 0 {
			durationMin = 1
		}
	}

	// invoca DeviceService se serve
	if durationMin > 0 {
		req := &pb.StartRequest{
			FieldId:     fieldID,
			SensorId:    sensorID,
			AmountMm:    doseMM,
			DurationMin: int32(durationMin),
		}
		if resp, err := device.StartIrrigation(ctx, req); err != nil {
			log.Printf("controller: StartIrrigation error: %v", err)
		} else if !resp.GetSuccess() {
			log.Printf("controller: StartIrrigation failed: %s", resp.GetMessage())
		}
	}

	// pubblica evento decisione (per EventService/Dashboard)
	return c.publishDecision(fieldID, sensorID, doseMM, float64(durationMin), float64(s.Moisture))
}

func (c *Controller) publishDecision(fieldID, sensorID string, doseMM, durMin, SMT float64) error {
	evt := model.IrrigationDecisionEvent{
		FieldID:        fieldID,
		SensorID:       sensorID,
		Stage:          "", // non c'è Stage disponibile nel modello attuale
		DrPct:          0,
		SMT:            SMT,
		DoseMM:         doseMM,
		RemainingToday: 0,
		Timestamp:      time.Now().UTC(),
	}
	b, _ := json.Marshal(evt)
	topic := strings.NewReplacer("{field}", fieldID, "{sensor}", sensorID).Replace(c.decisionTopicTmpl)
	if err := c.publisher.PublishTo(topic, string(b)); err != nil {
		log.Printf("controller: publish decision error: %v", err)
		return err
	}
	log.Printf("decision: %s/%s dose=%.1fmm dur=%.0fmin topic=%s", fieldID, sensorID, doseMM, durMin, topic)
	return nil
}

func (c *Controller) lookupSensor(fieldID, sensorID string) (model.Sensor, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if m, ok := c.sensors[fieldID]; ok {
		if s, ok2 := m[sensorID]; ok2 {
			return s, true
		}
	}
	return model.Sensor{}, false
}

// carica i sensori dal file JSON: { "field_1": [ {id, latitude, longitude, max_depth, flow_rate}, ... ], ... }
func loadSensors(path string) (map[string]map[string]model.Sensor, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string][]model.Sensor
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	out := make(map[string]map[string]model.Sensor, len(m))
	for fid, list := range m {
		inner := make(map[string]model.Sensor, len(list))
		for _, s := range list {
			if s.FieldID == "" {
				s.FieldID = fid
			}
			inner[s.ID] = s
		}
		out[fid] = inner
	}
	return out, nil
}

// mm/min = Lpm / m^2 (1 L/m^2 = 1 mm). Se area o flow mancano/0 → fallback 1 mm/min.
func flowMMPerMinute(s model.Sensor) float64 {
	area := s.AreaM2
	if area <= 0 {
		area = 1 // fallback: 1 m^2
	}
	flow := s.FlowLpm // json:"flow_rate"
	if flow <= 0 {
		return 1.0
	}
	return flow / area
}

func sensorLatitude(s model.Sensor) float64 {
	if s.Latitude != 0 {
		return s.Latitude
	}
	return 0
}

func sensorLongitude(s model.Sensor) float64 {
	if s.Longitude != 0 {
		return s.Longitude
	}
	return 0
}

//TODO il servizio da errore con l'API del tempo:  controller: weather error: owm status 401 (using defaults), SISTEMAREEEE
