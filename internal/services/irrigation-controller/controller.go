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

	// AGGIUNTE per dedup QoS1
	"crypto/sha256"
	"encoding/hex"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/dedup"

	pb "github.com/LeonardoBeccarini/sdcc_project/grpc/gen/go/irrigation"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// ===================== Config / defaults =====================

const (
	defaultTZ             = "Europe/Rome"
	defaultBaseMM         = 5.0 // DAILY_BASE_MM
	defaultEtoCoefficient = 0.5 // DAILY_ETO_COEFF
)

// ===================== Controller =====================

type Controller struct {
	consumer  rabbitmq.IConsumer[model.SensorData]
	publisher rabbitmq.IPublisher
	router    DeviceRouter
	wclient   WeatherClient
	sensors   map[string]map[string]model.Sensor // field -> sensorID -> Sensor

	// guard levels (percentuali), ordinati desc (es. 35,25)
	guardLevels []float64

	// parametri budget giornaliero
	baseMM   float64
	etoCoeff float64

	decisionTopicTmpl string

	// anti-doppi ON
	wateringMu    sync.Mutex
	wateringUntil map[string]time.Time // key = field|sensor

	// daily budget tracking
	tz             *time.Location
	dailyMu        sync.Mutex
	dailyDay       map[string]time.Time // giorno (mezzanotte locale) per cui è valido il budget
	dailyBudget    map[string]float64   // mm totali del giorno (solo informativo)
	dailyRemaining map[string]float64   // mm residui nel giorno

	// accesso sensori
	mu sync.RWMutex

	// ⬇️ NUOVO: deduper per scartare redelivery QoS1 (hash payload)
	deduper *dedup.Deduper
}

// DeviceRouter espone un client gRPC per ogni field (field -> DeviceService).
type DeviceRouter interface {
	Get(field string) (pb.DeviceServiceClient, bool)
	Close()
}

// WeatherClient restituisce ET0 e pioggia giornaliera (mm) per lat/lon/data.
type WeatherClient interface {
	GetDailyET0AndRain(ctx context.Context, lat, lon float64, day time.Time) (etoMM, rainMM float64, err error)
}

// ===================== ctor =====================

func NewController(
	c rabbitmq.IConsumer[model.SensorData],
	p rabbitmq.IPublisher,
	router DeviceRouter,
	wc WeatherClient,
	_ string, // fieldPolicyPath (unused, compat)
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

	// timezone
	tzName := strings.TrimSpace(os.Getenv("TZ"))
	if tzName == "" {
		tzName = defaultTZ
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		log.Printf("WARN: invalid TZ=%q, falling back to local: %v", tzName, err)
		loc = time.Local
	}

	// guard levels
	guards := parseGuards(os.Getenv("MOISTURE_GUARDS"))
	if len(guards) == 0 {
		// retro-compat: singola soglia
		th := 35.0
		if v := strings.TrimSpace(os.Getenv("MOISTURE_THRESHOLD_PCT")); v != "" {
			if f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64); err == nil && f > 0 {
				th = f
			}
		}
		guards = []float64{th}
	}
	// ordina desc (es. 35,25,15)
	sortDesc(guards)

	// budget params
	baseMM := getenvFloat("DAILY_BASE_MM", defaultBaseMM)
	etoCoeff := getenvFloat("DAILY_ETO_COEFF", defaultEtoCoefficient)

	ctrl := &Controller{
		consumer:          c,
		publisher:         p,
		router:            router,
		wclient:           wc,
		sensors:           sensors,
		guardLevels:       guards,
		baseMM:            baseMM,
		etoCoeff:          etoCoeff,
		decisionTopicTmpl: firstNonEmpty(decisionTopicTmpl, "event/irrigationDecision/{field}/{sensor}"),
		wateringUntil:     make(map[string]time.Time),
		tz:                loc,
		dailyDay:          make(map[string]time.Time),
		dailyBudget:       make(map[string]float64),
		dailyRemaining:    make(map[string]float64),
		// ⬇️ init deduper (TTL 10m, cap 20k)
		deduper: dedup.New(10*time.Minute, 20000),
	}
	c.SetHandler(ctrl.handleAggregated)
	return ctrl, nil
}

func (c *Controller) Start(ctx context.Context) {
	go c.consumer.ConsumeMessage(ctx)
	<-ctx.Done()
}

// ===================== handler dati aggregati =====================

func (c *Controller) handleAggregated(_ string, msg mqtt.Message) error {
	// ⬇️ DEDUP PRIMA DI UNMARSHAL: scarta redelivery QoS1 identiche
	h := sha256.Sum256(msg.Payload())
	if c.deduper != nil && !c.deduper.ShouldProcess(hex.EncodeToString(h[:])) {
		return nil
	}

	var s model.SensorData
	if err := json.Unmarshal(msg.Payload(), &s); err != nil {
		log.Printf("controller: bad payload: %v", err)
		return nil
	}
	if !s.Aggregated {
		return nil
	}

	fieldID, sensorID := s.FieldID, s.SensorID
	sensor, ok := c.lookupSensor(fieldID, sensorID)
	if !ok {
		log.Printf("controller: unknown sensor %s/%s", fieldID, sensorID)
		return nil
	}
	device, ok := c.router.Get(fieldID)
	if !ok {
		log.Printf("controller: no device client for field=%s", fieldID)
		return nil
	}

	// DEBUG: ingresso dato aggregato
	log.Printf("agg: %s/%s moisture=%d%% at=%s (flow=%.2flpm area=%.2fm2 guards=%v)",
		fieldID, sensorID, s.Moisture, s.Timestamp.UTC().Format(time.RFC3339), sensor.FlowLpm, sensor.AreaM2, c.guardLevels)

	// Assicura budget del giorno (calcolato una volta con meteo)
	dayStart := midnightLocal(time.Now(), c.tz)
	_, _, rem, err := c.ensureDailyBudget(context.Background(), sensor, dayStart) // scarto eto/rain
	if err != nil {
		log.Printf("controller: budget init error for %s/%s: %v", fieldID, sensorID, err)
	}
	log.Printf("budget: %s/%s day=%s remaining=%.2fmm", fieldID, sensorID, dayStart.Format("2006-01-02"), rem)

	// === LOGICA DI IRRIGAZIONE (INALTERATA) ===
	// Meteo per la formula dose esistente
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	eto, rain, wErr := c.wclient.GetDailyET0AndRain(ctx, sensor.Latitude, sensor.Longitude, time.Now().UTC())
	if wErr != nil {
		log.Printf("controller: weather error: %v (using defaults)", wErr)
		eto, rain = 4.0, 0.0
	}
	log.Printf("weather: %s/%s et0=%.2f rain=%.2f", fieldID, sensorID, eto, rain)

	// decisione semplice: se moisture (%) sotto soglia → irriga
	doseMM := 0.0
	preDose := 0.0
	moist := float64(s.Moisture)
	if belowAnyGuard(moist, c.guardLevels) {
		base := 5.0 // mm di base (come prima)
		etoAdj := math.Max(0, eto-rain)
		doseMM = base + 0.5*etoAdj
		preDose = doseMM
		log.Printf("decision-calc: %s/%s moist=%.1f%% < guards=%v → base=%.1f + 0.5*(%.2f-%.2f)=%.2f → preDose=%.2fmm", fieldID, sensorID, moist, c.guardLevels, base, eto, rain, etoAdj, preDose)
	} else {
		log.Printf("decision-skip: %s/%s moist=%.1f%% >= guards=%v → no irrigation", fieldID, sensorID, moist, c.guardLevels)
	}

	// *** UNICA AGGIUNTA: rispetta il budget residuo del giorno ***
	if doseMM > rem {
		log.Printf("budget-cap: %s/%s preDose=%.2fmm > remaining=%.2fmm → capped to %.2fmm", fieldID, sensorID, doseMM, rem, rem)
		doseMM = rem
	}

	// traduzione dose → minuti con flow/area
	durationMin := 0
	mmPerMin := sensor.MMPerMinute()
	if doseMM > 0 && mmPerMin > 0 {
		durationMin = int(math.Round(doseMM / mmPerMin))
		if durationMin <= 0 {
			durationMin = 1
		}
		log.Printf("duration: %s/%s dose=%.2fmm mmPerMin=%.4f → minutes=%d", fieldID, sensorID, doseMM, mmPerMin, durationMin)
	} else if doseMM > 0 && mmPerMin <= 0 {
		log.Printf("controller: mmPerMin<=0 per %s/%s (flow=%.2f lpm, area=%.2f m2) → skip avvio irrigazione",
			fieldID, sensor.ID, sensor.FlowLpm, sensor.AreaM2)
	}

	// evita di schedulare se è già ON
	if durationMin > 0 {
		now := time.Now()
		k := key(fieldID, sensorID)

		c.wateringMu.Lock()
		busyUntil, have := c.wateringUntil[k]
		if have && now.Before(busyUntil) {
			c.wateringMu.Unlock()
			log.Printf("controller: skip start %s/%s (già ON fino a %s)", fieldID, sensorID, busyUntil.Format(time.RFC3339))
			return nil
		}
		c.wateringMu.Unlock()
	}

	// invoca DeviceService se serve
	if durationMin > 0 {
		req := &pb.StartRequest{
			FieldId:     fieldID,
			SensorId:    sensorID,
			AmountMm:    doseMM,
			DurationMin: int32(durationMin),
		}
		rctx, rcancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer rcancel()

		if resp, err := device.StartIrrigation(rctx, req); err != nil {
			log.Printf("controller: StartIrrigation error: %v", err)
		} else if !resp.GetSuccess() {
			log.Printf("controller: StartIrrigation failed: %s", resp.GetMessage())
		} else {
			// avvio riuscito → aggiorna finestra busy-until
			until := time.Now().Add(time.Duration(durationMin) * time.Minute)
			k := key(fieldID, sensorID)
			c.wateringMu.Lock()
			if prev, ok := c.wateringUntil[k]; !ok || until.After(prev) {
				c.wateringUntil[k] = until
			}
			c.wateringMu.Unlock()

			// scala il residuo giornaliero
			c.deductBudget(k, dayStart, doseMM)
			log.Printf("controller: watering %s/%s ON per %d min (busy until %s)",
				fieldID, sensorID, durationMin, until.Format(time.RFC3339))
		}
	}

	// Pubblicazione evento decisione: invariata (solo se dose/durata > 0)
	if doseMM <= 0 || durationMin <= 0 {
		return nil
	}
	return c.publishDecision(fieldID, sensorID, doseMM, float64(durationMin), moist)
}

// ===================== Budget helpers =====================
// (immutati)

func (c *Controller) ensureDailyBudget(ctx context.Context, s model.Sensor, dayStart time.Time) (eto, rain, remaining float64, err error) {
	// ... (immutato)
	k := key(s.FieldID, s.ID)

	c.dailyMu.Lock()
	day, have := c.dailyDay[k]
	if have && day.Equal(dayStart) {
		rem := c.dailyRemaining[k]
		b := c.dailyBudget[k]
		eto = 0
		rain = 0
		log.Printf("budget: reuse %s/%s day=%s daily=%.2fmm remaining=%.2fmm", s.FieldID, s.ID, dayStart.Format("2006-01-02"), b, rem)
		c.dailyMu.Unlock()
		return eto, rain, rem, nil
	}
	c.dailyMu.Unlock()

	// Calcolo budget...
	eto, rain, err = c.wclient.GetDailyET0AndRain(ctx, s.Latitude, s.Longitude,
		time.Date(dayStart.Year(), dayStart.Month(), dayStart.Day(), 0, 0, 0, 0, time.UTC))
	if err != nil {
		log.Printf("controller: weather error for %s/%s: %v (fallback)", s.FieldID, s.ID, err)
		eto, rain = 4.0, 0.0
	}
	B := c.baseMM + c.etoCoeff*math.Max(0, eto-rain)
	if B < 0 {
		B = 0
	}

	c.dailyMu.Lock()
	c.dailyDay[k] = dayStart
	c.dailyBudget[k] = B
	c.dailyRemaining[k] = B
	log.Printf("budget: compute %s/%s day=%s et0=%.2f rain=%.2f base=%.2f coeff=%.2f → daily=%.2fmm", s.FieldID, s.ID, dayStart.Format("2006-01-02"), eto, rain, c.baseMM, c.etoCoeff, B)
	c.dailyMu.Unlock()

	return eto, rain, B, nil
}

func (c *Controller) deductBudget(k string, dayStart time.Time, applied float64) float64 {
	// ... (immutato)
	c.dailyMu.Lock()
	defer c.dailyMu.Unlock()

	day := c.dailyDay[k]
	if !day.Equal(dayStart) {
		c.dailyDay[k] = dayStart
		c.dailyBudget[k] = 0
		c.dailyRemaining[k] = 0
	}

	rem := c.dailyRemaining[k] - applied
	if rem < 0 {
		rem = 0
	}
	c.dailyRemaining[k] = rem
	fs := strings.ReplaceAll(k, "|", "/")
	log.Printf("budget: after apply %s applied=%.2fmm remaining=%.2fmm", fs, applied, rem)
	return rem
}

// ===================== Publish & utilities =====================

func (c *Controller) publishDecision(fieldID, sensorID string, doseMM, durMin, SMT float64) error {
	// evita pubblicazioni "0 mm / 0 min" (logica inalterata)
	if doseMM <= 0 || durMin <= 0 {
		return nil
	}

	evt := model.IrrigationDecisionEvent{
		FieldID:        fieldID,
		SensorID:       sensorID,
		Stage:          "", // invariante rispetto alla pipeline attuale
		DrPct:          0,
		SMT:            SMT,
		DoseMM:         doseMM,
		RemainingToday: 0, // lasciato invariato nella pubblicazione
		Timestamp:      time.Now().UTC(),
	}
	b, _ := json.Marshal(evt)
	topic := strings.NewReplacer("{field}", fieldID, "{sensor}", sensorID).Replace(c.decisionTopicTmpl)

	// ⬇️ UNICA MODIFICA QUI: publish decision a QoS=1
	if err := c.publisher.PublishToQos(topic, 1, false, string(b)); err != nil {
		log.Printf("controller: publish decision error: %v", err)
		return err
	}
	log.Printf("decision: %s/%s dose=%.1fmm dur=%.0fmin topic=%s (qos=1)", fieldID, sensorID, doseMM, durMin, topic)
	return nil
}

// ... resto del file (helpers invariati) ...

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

// carica i sensori dal file JSON accettando sia "flow_lpm" sia "flow_rate".
func loadSensors(path string) (map[string]map[string]model.Sensor, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// leggo come map[string][]map[string]any così posso gestire alias dei campi
	var m map[string][]map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}

	out := make(map[string]map[string]model.Sensor, len(m))
	for fid, list := range m {
		inner := make(map[string]model.Sensor, len(list))
		for _, rec := range list {
			var s model.Sensor
			// id e field
			if v, ok := rec["id"].(string); ok {
				s.ID = v
			}
			if s.ID == "" {
				return nil, fmt.Errorf("sensor without id in field %s", fid)
			}
			s.FieldID = fid

			// geo/depth
			s.Latitude = toF64(rec["latitude"])
			s.Longitude = toF64(rec["longitude"])
			if md, ok := rec["max_depth"]; ok {
				s.MaxDepth = int(toF64(md))
			}

			// area
			s.AreaM2 = toF64(rec["area_m2"])

			// portata: preferisci flow_lpm, altrimenti flow_rate
			flow := toF64(rec["flow_lpm"])
			if flow == 0 {
				flow = toF64(rec["flow_rate"])
			}
			s.FlowLpm = flow

			inner[s.ID] = s
		}
		out[fid] = inner
	}
	return out, nil
}

// helper per convertire interi/float/string -> float64
func toF64(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int32:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		if f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(t), ",", "."), 64); err == nil {
			return f
		}
	}
	return 0
}

// --------------------- small helpers ---------------------

func key(fid, sid string) string { return fid + "|" + sid }

func getenvFloat(k string, def float64) float64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
	if err != nil {
		return def
	}
	return f
}

func midnightLocal(t time.Time, loc *time.Location) time.Time {
	lt := t.In(loc)
	return time.Date(lt.Year(), lt.Month(), lt.Day(), 0, 0, 0, 0, loc)
}

func parseGuards(s string) []float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []float64
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		f, err := strconv.ParseFloat(strings.ReplaceAll(p, ",", "."), 64)
		if err == nil && f > 0 {
			out = append(out, f)
		}
	}
	return out
}

func sortDesc(a []float64) {
	for i := 0; i < len(a); i++ {
		for j := i + 1; j < len(a); j++ {
			if a[j] > a[i] {
				a[i], a[j] = a[j], a[i]
			}
		}
	}
}

func belowAnyGuard(moist float64, guards []float64) bool {
	for _, g := range guards {
		if moist < g {
			return true
		}
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
