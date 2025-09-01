package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	// modelli dal progetto
	ents "github.com/LeonardoBeccarini/sdcc_project/internal/model/entities"

	// circuit breaker
	"github.com/sony/gobreaker"
)

/************* MODELS (DTO verso la dashboard) *************/
// NB: usiamo entities.SensorState per lo status
type Sensor struct {
	ID     string           `json:"id"`
	Value  float64          `json:"value"`
	Status ents.SensorState `json:"status"`
}

type Irrigation struct {
	SensorID string `json:"sensor_id"`
	Amount   int    `json:"amount"` // dose in mm (arrotondata)
	Time     string `json:"time"`   // RFC3339
}

type Stats struct {
	Mean float64 `json:"mean"`
	Max  float64 `json:"max"`
	Min  float64 `json:"min"`
}

type Payload struct {
	Sensors     []Sensor     `json:"sensors"`
	Irrigations []Irrigation `json:"irrigations"`
	Stats       Stats        `json:"stats"`
}

/************* UPSTREAM REST CLIENT *************/
type Upstream struct {
	http    *http.Client
	timeout time.Duration
}

func NewUpstream(timeoutMs int) *Upstream {
	return &Upstream{
		http:    &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond},
		timeout: time.Duration(timeoutMs) * time.Millisecond,
	}
}

func (u *Upstream) getJSON(ctx context.Context, url string, out any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	res, err := u.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("GET %s -> %s", url, res.Status)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// Device-Service: restituisce map[sensorID]state
func (u *Upstream) GetStatuses(ctx context.Context, base string) map[string]ents.SensorState {
	type raw map[string]string
	var r raw
	_ = u.getJSON(ctx, base+"/sensors/status", &r)

	out := make(map[string]ents.SensorState, len(r))
	for k, v := range r {
		out[k] = ents.SensorState(v)
	}
	return out
}

// Persistence-Service: restituisce []Sensor (id,value)
func (u *Upstream) GetValues(ctx context.Context, base string) []Sensor {
	var out []Sensor
	_ = u.getJSON(ctx, base+"/data/latest", &out)
	return out
}

// Event-Service: restituisce []Irrigation (DTO semplice)
func (u *Upstream) GetEvents(ctx context.Context, base string) ([]Irrigation, error) {
	var out []Irrigation
	if err := u.getJSON(ctx, base+"/irrigations/recent", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Analytics-Service: restituisce Stats
func (u *Upstream) GetStats(ctx context.Context, base string) Stats {
	var out Stats
	_ = u.getJSON(ctx, base+"/analytics/stats", &out)
	return out
}

/************* HELPERS *************/
func applyStatus(values []Sensor, statuses map[string]ents.SensorState) []Sensor {
	if len(values) == 0 {
		return nil
	}
	m := make(map[string]ents.SensorState, len(statuses))
	for k, v := range statuses {
		m[k] = v
	}
	out := make([]Sensor, 0, len(values))
	for _, s := range values {
		s.Status = m[s.ID]
		out = append(out, s)
	}
	return out
}

/************* MAIN *************/
var (
	eventCB       *gobreaker.CircuitBreaker
	deviceCB      *gobreaker.CircuitBreaker
	persistenceCB *gobreaker.CircuitBreaker
	analyticsCB   *gobreaker.CircuitBreaker

	lastGoodEvents []Irrigation
)

func mkCB(name string, fails, openMs, intervalMs int) *gobreaker.CircuitBreaker {
	return gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:     name,
		Interval: time.Duration(intervalMs) * time.Millisecond,
		Timeout:  time.Duration(openMs) * time.Millisecond,
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.ConsecutiveFailures >= uint32(fails)
		},
	})
}

func main() {
	cfg := loadConfig()

	// Circuit breakers
	eventCB = mkCB("event-service", cfg.CBEventFails, cfg.CBEventOpenMs, cfg.CBEventIntervalMs)
	deviceCB = mkCB("device-service", cfg.CBRestFails, cfg.CBRestOpenMs, cfg.CBRestIntervalMs)
	persistenceCB = mkCB("persistence-service", cfg.CBRestFails, cfg.CBRestOpenMs, cfg.CBRestIntervalMs)
	analyticsCB = mkCB("analytics-service", cfg.CBRestFails, cfg.CBRestOpenMs, cfg.CBRestIntervalMs)

	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	http.HandleFunc("/dashboard/data", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.TimeoutMs)*time.Millisecond)
		defer cancel()

		up := NewUpstream(cfg.TimeoutMs)

		// === (1) IRRIGATIONS: REST con Circuit Breaker (no Influx fallback) ===
		var irrigations []Irrigation
		if res, err := eventCB.Execute(func() (any, error) {
			ev, err := up.GetEvents(ctx, cfg.EventURL)
			if err != nil || len(ev) == 0 {
				if err == nil {
					err = fmt.Errorf("empty events")
				}
				return nil, err
			}
			return ev, nil
		}); err == nil {
			irrigations = res.([]Irrigation)
			lastGoodEvents = irrigations
		} else {
			// usa l'ultima cache valida (se presente)
			irrigations = lastGoodEvents
		}

		// === (2) SENSORS / STATS: ONLY REST (ognuno con il proprio CB) ===
		var (
			statuses map[string]ents.SensorState
			values   []Sensor
			stats    Stats
		)

		if res, err := deviceCB.Execute(func() (any, error) {
			m := up.GetStatuses(ctx, cfg.DeviceURL)
			if len(m) == 0 {
				return nil, fmt.Errorf("empty statuses")
			}
			return m, nil
		}); err == nil {
			statuses = res.(map[string]ents.SensorState)
		}

		if res, err := persistenceCB.Execute(func() (any, error) {
			v := up.GetValues(ctx, cfg.PersistenceURL)
			if len(v) == 0 {
				return nil, fmt.Errorf("empty values")
			}
			return v, nil
		}); err == nil {
			values = res.([]Sensor)
		}

		if res, err := analyticsCB.Execute(func() (any, error) {
			s := up.GetStats(ctx, cfg.AnalyticsURL)
			if s == (Stats{}) {
				return nil, fmt.Errorf("empty stats")
			}
			return s, nil
		}); err == nil {
			stats = res.(Stats)
		}

		sensors := applyStatus(values, statuses)

		resp := Payload{
			Sensors:     sensors,
			Irrigations: irrigations,
			Stats:       stats,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)

		log.Printf("GET /dashboard/data [%dms] cb[event]=%v cb[dev]=%v cb[pers]=%v cb[an]=%v sensors=%d events=%d",
			time.Since(start).Milliseconds(), eventCB.State(), deviceCB.State(), persistenceCB.State(), analyticsCB.State(),
			len(resp.Sensors), len(resp.Irrigations))
	})

	addr := ":" + cfg.Port
	log.Printf("gateway listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
