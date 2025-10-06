package irrigation_controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/LeonardoBeccarini/sdcc_project/pkg/dedup"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// ===================== Config / defaults =====================

const (
	defaultTZ             = "Europe/Rome"
	defaultBaseMM         = 5.0   // DAILY_BASE_MM
	defaultEtoCoefficient = 0.5   // DAILY_ETO_COEFF
	defaultPendingMargin  = "5m"  // IRR_PENDING_TTL_MARGIN
	defaultDeduperTTL     = "10m" // DEDUP_TTL
)

// ===================== Controller =====================

type Controller struct {
	consumer rabbitmq.IConsumer[model.SensorData]
	router   DeviceRouter
	wclient  WeatherClient
	sensors  map[string]map[string]model.Sensor // field -> sensorID -> Sensor

	guardLevels []float64 // percentuali, desc

	baseMM   float64
	etoCoeff float64

	// anti-doppi ON
	wateringMu    sync.Mutex
	wateringUntil map[string]time.Time // key = field|sensor

	// daily budget tracking
	tz             *time.Location
	dailyMu        sync.Mutex
	dailyDay       map[string]time.Time
	dailyBudget    map[string]float64
	dailyRemaining map[string]float64

	mu sync.RWMutex // accesso mappa sensori

	// dedup input (aggregated)
	deduper *dedup.Deduper

	// ==== gestione Result ====
	resultConsumer rabbitmq.IConsumer[model.IrrigationResultEvent]
	pending        sync.Map // ticket_id -> pendingEntry
	marginTTL      time.Duration
}

type pendingEntry struct {
	FieldID    string
	SensorID   string
	DayStart   time.Time
	ExpectedMM float64
	Deadline   time.Time
}

// DeviceRouter e WeatherClient:
type DeviceRouter interface {
	Get(field string) (pb.DeviceServiceClient, bool)
	Close()
}
type WeatherClient interface {
	GetDailyET0AndRain(ctx context.Context, lat, lon float64, day time.Time) (etoMM, rainMM float64, err error)
}

// ===================== controller =====================

func NewController(
	c rabbitmq.IConsumer[model.SensorData],
	router DeviceRouter,
	wc WeatherClient,
	_ string, // fieldPolicyPath (unused, compat)
	sensorsPath string,
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
		log.Printf("WARN: invalid TZ=%q, using local: %v", tzName, err)
		loc = time.Local
	}

	// guard levels
	guards := parseGuards(os.Getenv("MOISTURE_GUARDS"))
	if len(guards) == 0 {
		th := 35.0
		if v := strings.TrimSpace(os.Getenv("MOISTURE_THRESHOLD_PCT")); v != "" {
			if f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64); err == nil && f > 0 {
				th = f
			}
		}
		guards = []float64{th}
	}
	sortDesc(guards)

	// budget params
	baseMM := getenvFloat("DAILY_BASE_MM", defaultBaseMM)
	etoCoeff := getenvFloat("DAILY_ETO_COEFF", defaultEtoCoefficient)

	//deduper TTL
	dTTL := getenvDuration("DEDUPER_TTL", defaultDeduperTTL)

	// pending TTL
	mTTL := getenvDuration("IRR_PENDING_MARGIN", defaultPendingMargin)

	ctrl := &Controller{
		consumer:       c,
		router:         router,
		wclient:        wc,
		sensors:        sensors,
		guardLevels:    guards,
		baseMM:         baseMM,
		etoCoeff:       etoCoeff,
		wateringUntil:  make(map[string]time.Time),
		tz:             loc,
		dailyDay:       make(map[string]time.Time),
		dailyBudget:    make(map[string]float64),
		dailyRemaining: make(map[string]float64),
		deduper:        dedup.New(dTTL, 20000),
		marginTTL:      mTTL, // margine da aggiungere alla durata dell'irrigazione prima di decurtare il budget
	}
	c.SetHandler(ctrl.handleAggregated)
	return ctrl, nil
}

// AttachResultConsumer permette di collegare il consumer su event/irrigationResult/# senza cambiare il costruttore.
func (c *Controller) AttachResultConsumer(rc rabbitmq.IConsumer[model.IrrigationResultEvent]) {
	c.resultConsumer = rc
	if c.resultConsumer != nil {
		c.resultConsumer.SetHandler(c.handleIrrigationResult)
	}
}

func (c *Controller) Start(ctx context.Context) {
	// dati aggregati (già esistente)
	go c.consumer.ConsumeMessage(ctx)
	// risultati irrigazione (nuovo, opzionale)
	if c.resultConsumer != nil {
		go c.resultConsumer.ConsumeMessage(ctx)
	}
	// GC pending (timeout)
	go c.gcPending(ctx)
	<-ctx.Done()
}

// ===================== handler dati aggregati =====================

func (c *Controller) handleAggregated(_ string, msg mqtt.Message) error {
	// dedup QoS1
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

	log.Printf("agg: %s/%s moisture=%d%% at=%s (flow=%.2flpm area=%.2fm2 guards=%v)",
		fieldID, sensorID, s.Moisture, s.Timestamp.UTC().Format(time.RFC3339), sensor.FlowLpm, sensor.AreaM2, c.guardLevels)

	dayStart := midnightLocal(time.Now(), c.tz)
	_, _, rem, err := c.ensureDailyBudget(context.Background(), sensor, dayStart)
	if err != nil {
		log.Printf("controller: budget init error for %s/%s: %v", fieldID, sensorID, err)
	}
	log.Printf("budget: %s/%s day=%s remaining=%.2fmm", fieldID, sensorID, dayStart.Format("2006-01-02"), rem)

	// LOGICA di decisione
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	eto, rain, wErr := c.wclient.GetDailyET0AndRain(ctx, sensor.Latitude, sensor.Longitude, time.Now().UTC())
	if wErr != nil {
		log.Printf("controller: weather error: %v (using defaults)", wErr)
		eto, rain = 4.0, 0.0
	}
	log.Printf("weather: %s/%s et0=%.2f rain=%.2f", fieldID, sensorID, eto, rain)

	doseMM := 0.0
	preDose := 0.0
	moist := float64(s.Moisture)
	if belowAnyGuard(moist, c.guardLevels) {
		base := 5.0
		etoAdj := math.Max(0, eto-rain)
		doseMM = base + 0.5*etoAdj
		preDose = doseMM
		etoTerm := 0.5 * etoAdj // solo per log
		log.Printf("decision-calc: %s/%s moist=%.1f%% < guards=%v → base=%.1f + 0.5*max(0, %.2f-%.2f)=%.2f → preDose=%.2fmm",
			fieldID, sensorID, moist, c.guardLevels, base, eto, rain, etoTerm, preDose)

	} else {
		log.Printf("decision-skip: %s/%s moist=%.1f%% >= guards=%v → no irrigation", fieldID, sensorID, moist, c.guardLevels)
	}

	// rispetto budget giornaliero
	if doseMM > rem {
		log.Printf("budget-cap: %s/%s preDose=%.2fmm > remaining=%.2fmm → capped to %.2fmm", fieldID, sensorID, doseMM, rem, rem)
		doseMM = rem
	}

	// mm→min
	durationMin := 0
	mmPerMin := mmPerMinute(sensor)
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

	// evita doppio ON
	if durationMin > 0 {
		now := time.Now()
		k := key(fieldID, sensorID)
		c.wateringMu.Lock()
		if until, have := c.wateringUntil[k]; have && now.Before(until) {
			c.wateringMu.Unlock()
			log.Printf("controller: skip start %s/%s (già ON fino a %s)", fieldID, sensorID, until.Format(time.RFC3339))
			return nil
		}
		c.wateringMu.Unlock()
	}

	// chiamata gRPC
	if durationMin > 0 {
		req := &pb.StartRequest{
			FieldId:     fieldID,
			SensorId:    sensorID,
			AmountMm:    doseMM,
			DurationMin: int32(durationMin),
			DecisionId:  fmt.Sprintf("%s|%s|%d", fieldID, sensorID, time.Now().UnixNano()),
		}
		rctx, rcancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer rcancel()

		if resp, err := device.StartIrrigation(rctx, req); err != nil {
			log.Printf("controller: StartIrrigation error: %v", err)
		} else if !resp.GetSuccess() {
			log.Printf("controller: StartIrrigation failed: %s", resp.GetMessage())
		} else {
			// busy until per evitare doppio ON
			until := time.Now().Add(time.Duration(durationMin) * time.Minute)
			k := key(fieldID, sensorID)
			c.wateringMu.Lock()
			if prev, ok := c.wateringUntil[k]; !ok || until.After(prev) {
				c.wateringUntil[k] = until
			}
			c.wateringMu.Unlock()

			// niente decremento immediato
			tid := strings.TrimSpace(resp.GetTicketId())
			if tid != "" {
				// TTL per-ticket = durata + margine
				ttlTotal := time.Duration(durationMin)*time.Minute + c.marginTTL

				c.pending.Store(tid, pendingEntry{
					FieldID:    fieldID,
					SensorID:   sensorID,
					DayStart:   dayStart,
					ExpectedMM: doseMM,
					Deadline:   time.Now().Add(ttlTotal),
				})

				log.Printf("controller: pending add ticket=%s %s/%s expected=%.2fmm ttl=%s (dur=%dm + margin=%s)",
					tid, fieldID, sensorID, doseMM, ttlTotal, durationMin, c.marginTTL)
			} else {
				log.Printf("controller: warning: empty ticket_id from device")
			}

			log.Printf("controller: watering %s/%s ON per %d min (busy until %s)",
				fieldID, sensorID, durationMin, until.Format(time.RFC3339))
		}
	}

	return nil
}

// ===================== Risultati irrigazione =====================

func (c *Controller) handleIrrigationResult(_ string, m mqtt.Message) error {
	var r model.IrrigationResultEvent
	if err := json.Unmarshal(m.Payload(), &r); err != nil {
		log.Printf("controller: bad result payload: %v", err)
		return nil
	}
	tid := strings.TrimSpace(r.TicketID)
	if tid == "" {
		return nil
	}

	k := key(r.FieldID, r.SensorID)

	// dedupe su ticket_id (QoS1)
	if c.deduper != nil && !c.deduper.ShouldProcess(tid) {
		log.Printf("result-dup: ticket=%s %s ignored (TTL)", tid, k)
		return nil
	}

	// Prendi (ed elimina) il pending per questo ticket, se c'è
	var day time.Time
	if v, ok := c.pending.LoadAndDelete(tid); ok {
		pe := v.(pendingEntry)
		day = pe.DayStart
		log.Printf("pending-del: ticket=%s %s/%s expected=%.2fmm deadline=%s",
			tid, pe.FieldID, pe.SensorID, pe.ExpectedMM, pe.Deadline.In(c.tz).Format(time.RFC3339))
		if diff := r.MmApplied - pe.ExpectedMM; math.Abs(diff) > 1e-6 {
			log.Printf("result-applied!=expected: ticket=%s expected=%.2fmm applied=%.2fmm Δ=%.2fmm",
				tid, pe.ExpectedMM, r.MmApplied, diff)
		}
	} else {
		// pending non trovato (es. timeout/riavvio) → fallback al giorno corrente
		day = midnightLocal(time.Now(), c.tz)
		log.Printf("pending-miss: ticket=%s %s using day=%s (no pending found)",
			tid, k, day.Format("2006-01-02"))
	}

	applied := r.MmApplied
	okStatus := strings.EqualFold(r.Status, "OK")
	failWithWater := strings.EqualFold(r.Status, "FAIL") && applied > 0

	if okStatus || failWithWater {
		// --- LOG PRE: fotografia prima della detrazione ---
		if remBefore, ok := c.peekRemainingByKey(k); ok {
			log.Printf("budget-deduct:pre %s day=%s applied=%.2fmm remaining_before=%.2fmm ticket=%s",
				k, day.Format("2006-01-02"), applied, remBefore, tid)
		} else {
			log.Printf("budget-deduct:pre %s day=%s applied=%.2fmm (no remaining snapshot) ticket=%s",
				k, day.Format("2006-01-02"), applied, tid)
		}

		// Detrazione atomica del budget del giorno
		c.deductBudget(k, day, applied)

		// --- LOG POST: fotografia dopo la detrazione ---
		if remAfter, ok := c.peekRemainingByKey(k); ok {
			log.Printf("budget-deduct:post %s day=%s remaining_after=%.2fmm",
				k, day.Format("2006-01-02"), remAfter)
		}

		log.Printf("result-commit: ticket=%s %s status=%s applied=%.2fmm",
			tid, k, r.Status, applied)
	} else {
		// FAIL senza acqua erogata o altri stati → nessuna variazione budget
		log.Printf("result-rollback: ticket=%s %s status=%s applied=%.2fmm → no budget change",
			tid, k, r.Status, applied)
	}

	return nil
}

func (c *Controller) gcPending(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			now := time.Now()
			c.pending.Range(func(k, v any) bool {
				pe := v.(pendingEntry)
				if now.After(pe.Deadline) {
					c.pending.Delete(k)
					log.Printf("controller: pending expired ticket=%s %s/%s", k, pe.FieldID, pe.SensorID)
				}
				return true
			})
		}
	}
}

// ==== helpers (lookup, loadSensors, conversions, math, etc.) ====

// Peek del remaining (read-only) a partire dalla chiave field|sensor.
// serve solo per log "pre/post".
func (c *Controller) peekRemainingByKey(k string) (float64, bool) {
	c.dailyMu.Lock()
	defer c.dailyMu.Unlock()
	v, ok := c.dailyRemaining[k]
	return v, ok
}

// ensureDailyBudget inizializza (o riapre) il budget giornaliero per il sensore/field
// alla mezzanotte "dayStart" e restituisce: budget del giorno, consumato e rimanente.
// Logica: budget = baseMM + etoCoeff * max(0, et0 - rain).
func (c *Controller) ensureDailyBudget(ctx context.Context, s model.Sensor, dayStart time.Time) (dayBudget, consumed, remaining float64, err error) {
	k := key(s.FieldID, s.ID)

	c.dailyMu.Lock()
	defer c.dailyMu.Unlock()

	lastDay, have := c.dailyDay[k]
	if !have || !lastDay.Equal(dayStart) {
		// Nuovo giorno: ricalcola il budget
		var eto, rain float64
		if c.wclient != nil {
			eto, rain, err = c.wclient.GetDailyET0AndRain(ctx, s.Latitude, s.Longitude, dayStart)
			if err != nil {
				// Non fallire la logica: usa 0/0 se meteo non disponibile
				eto, rain = 0, 0
			}
		}
		etoAdj := math.Max(0, eto-rain)
		budget := c.baseMM + c.etoCoeff*etoAdj
		if budget < 0 {
			budget = 0
		}

		c.dailyDay[k] = dayStart
		c.dailyBudget[k] = budget
		c.dailyRemaining[k] = budget
		etoTerm := c.etoCoeff * etoAdj // solo per log
		log.Printf("budget-init: %s/%s day=%s budget=%.2fmm (base=%.2f + %.2f*max(0, et0=%.2f - rain=%.2f)=%.2f)",
			s.FieldID, s.ID, dayStart.Format("2006-01-02"),
			budget, c.baseMM, c.etoCoeff, eto, rain, etoTerm)

	}

	dayBudget = c.dailyBudget[k]
	remaining = c.dailyRemaining[k]
	consumed = dayBudget - remaining
	return dayBudget, consumed, remaining, nil
}

// deductBudget scala in modo atomico il budget rimanente del giorno per (field|sensor).
func (c *Controller) deductBudget(key string, dayStart time.Time, mm float64) {
	if mm <= 0 {
		return
	}

	c.dailyMu.Lock()
	defer c.dailyMu.Unlock()

	// Se per qualche motivo il giorno non è inizializzato, inizializzalo in modo conservativo.
	if lastDay, ok := c.dailyDay[key]; !ok || !lastDay.Equal(dayStart) {
		c.dailyDay[key] = dayStart
		if _, ok := c.dailyBudget[key]; !ok {
			c.dailyBudget[key] = 0
		}
		if _, ok := c.dailyRemaining[key]; !ok {
			c.dailyRemaining[key] = c.dailyBudget[key]
		}
	}

	newRemaining := c.dailyRemaining[key] - mm
	if newRemaining < 0 {
		newRemaining = 0
	}
	c.dailyRemaining[key] = newRemaining
}

func getenvDuration(k, def string) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		v = def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return time.Minute * 5
	}
	return d
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

func loadSensors(path string) (map[string]map[string]model.Sensor, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var m map[string][]map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}

	out := make(map[string]map[string]model.Sensor, len(m))
	for fid, list := range m {
		inner := make(map[string]model.Sensor, len(list))
		for _, rec := range list {
			var s model.Sensor
			if v, ok := rec["id"].(string); ok {
				s.ID = v
			}
			if s.ID == "" {
				return nil, fmt.Errorf("sensor without id in field %s", fid)
			}
			s.FieldID = fid
			s.Latitude = toF64(rec["latitude"])
			s.Longitude = toF64(rec["longitude"])
			if md, ok := rec["max_depth"]; ok {
				s.MaxDepth = int(toF64(md))
			}
			s.AreaM2 = toF64(rec["area_m2"])
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

// mmPerMinute: 1 L/m2 = 1 mm
func mmPerMinute(s model.Sensor) float64 {
	if s.AreaM2 <= 0 || s.FlowLpm <= 0 {
		return 0
	}
	return s.FlowLpm / s.AreaM2
}
