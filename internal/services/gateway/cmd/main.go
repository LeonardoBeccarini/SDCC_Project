package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/sony/gobreaker"
)

// ======== Modelli ========
type Sensor struct {
	ID     string  `json:"id"`
	Value  float64 `json:"value"`
	Status string  `json:"status"`
}

type Irrigation struct {
	SensorID string `json:"sensor_id"`
	Amount   int    `json:"amount"`
	Time     string `json:"time"`
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

// ======== Config ========
type Config struct {
	DeviceURL      string
	PersistenceURL string
	EventURL       string
	AnalyticsURL   string

	Port           string
	Timeout        time.Duration // HTTP client timeout per chiamata
	BreakerFails   int           // quante failure consecutive per aprire il CB
	BreakerOpenSec int           // quanto resta Open prima di Half-Open
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
func getenvInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
func loadConfig() Config {
	return Config{
		DeviceURL:      getenv("DEVICE_URL", "http://localhost:5002"),
		PersistenceURL: getenv("PERSISTENCE_URL", "http://localhost:5000"),
		EventURL:       getenv("EVENT_URL", "http://localhost:5001"),
		AnalyticsURL:   getenv("ANALYTICS_URL", "http://localhost:5003"),
		Port:           getenv("PORT", "5009"),
		Timeout:        time.Duration(getenvInt("TIMEOUT_MS", 3000)) * time.Millisecond,
		BreakerFails:   getenvInt("BREAKER_FAILS", 5),
		BreakerOpenSec: getenvInt("BREAKER_OPEN_SECS", 15),
	}
}

// ======== HTTP utils ========
func getJSON(cli *http.Client, ctx context.Context, url string, out any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	res, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 500 {
		return errors.New("upstream 5xx")
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func applyStatus(vals []Sensor, statuses map[string]string) []Sensor {
	for i := range vals {
		if s, ok := statuses[vals[i].ID]; ok {
			vals[i].Status = s
		} else {
			vals[i].Status = "unknown"
		}
	}
	return vals
}

// ======== Client con Circuit Breaker + cache last-good ========
type CBClient struct {
	http *http.Client

	cbDevice    *gobreaker.CircuitBreaker
	cbPersist   *gobreaker.CircuitBreaker
	cbEvent     *gobreaker.CircuitBreaker
	cbAnalytics *gobreaker.CircuitBreaker

	mu           sync.RWMutex
	lastStatuses map[string]string
	lastValues   []Sensor
	lastEvents   []Irrigation
	lastStats    Stats
}

func newCBClient(timeout time.Duration, failThreshold int, openSecs int) *CBClient {
	mk := func(name string) *gobreaker.CircuitBreaker {
		st := gobreaker.Settings{
			Name:        name,
			Interval:    60 * time.Second,                      // finestra statistica
			Timeout:     time.Duration(openSecs) * time.Second, // Open -> Half-Open
			MaxRequests: 1,                                     // probe in Half-Open
			ReadyToTrip: func(c gobreaker.Counts) bool {
				return c.ConsecutiveFailures >= uint32(failThreshold)
			},
		}
		return gobreaker.NewCircuitBreaker(st)
	}
	return &CBClient{
		http:         &http.Client{Timeout: timeout},
		cbDevice:     mk("device"),
		cbPersist:    mk("persistence"),
		cbEvent:      mk("event"),
		cbAnalytics:  mk("analytics"),
		lastStatuses: map[string]string{},
	}
}

func (c *CBClient) GetStatuses(ctx context.Context, base string) map[string]string {
	call := func() (interface{}, error) {
		var s map[string]string
		if err := getJSON(c.http, ctx, base+"/sensors/status", &s); err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.lastStatuses = s
		c.mu.Unlock()
		return s, nil
	}
	v, err := c.cbDevice.Execute(call)
	if err != nil {
		// fallback: ultimo valore buono oppure vuoto
		c.mu.RLock()
		defer c.mu.RUnlock()
		out := make(map[string]string, len(c.lastStatuses))
		for k, v := range c.lastStatuses {
			out[k] = v
		}
		return out
	}
	return v.(map[string]string)
}

func (c *CBClient) GetValues(ctx context.Context, base string) []Sensor {
	call := func() (interface{}, error) {
		var vals []Sensor
		if err := getJSON(c.http, ctx, base+"/data/latest", &vals); err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.lastValues = vals
		c.mu.Unlock()
		return vals, nil
	}
	v, err := c.cbPersist.Execute(call)
	if err != nil {
		c.mu.RLock()
		defer c.mu.RUnlock()
		return append([]Sensor(nil), c.lastValues...)
	}
	return v.([]Sensor)
}

func (c *CBClient) GetEvents(ctx context.Context, base string) []Irrigation {
	call := func() (interface{}, error) {
		var ev []Irrigation
		if err := getJSON(c.http, ctx, base+"/irrigations/recent", &ev); err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.lastEvents = ev
		c.mu.Unlock()
		return ev, nil
	}
	v, err := c.cbEvent.Execute(call)
	if err != nil {
		c.mu.RLock()
		defer c.mu.RUnlock()
		return append([]Irrigation(nil), c.lastEvents...)
	}
	return v.([]Irrigation)
}

func (c *CBClient) GetStats(ctx context.Context, base string) Stats {
	call := func() (interface{}, error) {
		var st Stats
		if err := getJSON(c.http, ctx, base+"/analytics/stats", &st); err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.lastStats = st
		c.mu.Unlock()
		return st, nil
	}
	v, err := c.cbAnalytics.Execute(call)
	if err != nil {
		c.mu.RLock()
		defer c.mu.RUnlock()
		return c.lastStats
	}
	return v.(Stats)
}

// ======== main / HTTP server ========
func main() {
	cfg := loadConfig()
	cli := newCBClient(cfg.Timeout, cfg.BreakerFails, cfg.BreakerOpenSec)

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	http.HandleFunc("/dashboard/data", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), cfg.Timeout)
		defer cancel()

		// chiamate (semplici, in serie; puoi parallelizzarle più avanti con errgroup)
		statuses := cli.GetStatuses(ctx, cfg.DeviceURL)
		values := cli.GetValues(ctx, cfg.PersistenceURL)
		events := cli.GetEvents(ctx, cfg.EventURL)
		stats := cli.GetStats(ctx, cfg.AnalyticsURL)

		resp := Payload{
			Sensors:     applyStatus(values, statuses),
			Irrigations: events,
			Stats:       stats,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)

		log.Printf("GET /dashboard/data latency=%dms sensors=%d events=%d",
			time.Since(start).Milliseconds(), len(resp.Sensors), len(resp.Irrigations))
	})

	addr := ":" + cfg.Port
	log.Printf("API Gateway (CB-enabled) in ascolto su %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
