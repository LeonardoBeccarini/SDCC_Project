package sensor_simulator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
)

// ====== Tunables ======
const (
	// gainPerMin: +0.6% per minuto quando la valvola è ON (in [0..1]).
	gainPerMin = 0.006

	// defaultSeed: valore di seed se SoilGrids non è disponibile.
	defaultSeed = 0.30 // 30%

	// soilGridsURL: fetch singola all'avvio; NON chiamare ad ogni tick.
	soilGridsURL = "https://rest.isric.org/soilgrids/v2.0/properties/query?lat=%f&lon=%f&property=wv0010"
)

// DataGenerator mantiene lo stato interno della moisture e lo aggiorna nel tempo.
// Esegue al massimo UNA fetch opzionale a SoilGrids in fase di startup.
type DataGenerator struct {
	mu           sync.Mutex
	seeded       bool
	last         time.Time
	moisture     float64 // [0..1]
	decayPerMin  float64 // es. 0.001 → -0.1%/min quando OFF
	pendingBoost float64
	httpClient   *http.Client
}

// NewDataGenerator crea un generatore con dato tasso di decadimento (OFF) per minuto.
func NewDataGenerator(decayPerMin float64) *DataGenerator {
	return &DataGenerator{
		decayPerMin: math.Max(0, decayPerMin),
		httpClient:  &http.Client{Timeout: 8 * time.Second},
	}
}

// SeedFromSoilGrids --> singola fetch a SoilGrids all'avvio.
// Se fallisce, usa un seed di default (30%).
func (g *DataGenerator) SeedFromSoilGrids(ctx context.Context, s *model.Sensor) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.seeded {
		return
	}

	seed := defaultSeed
	if s.Latitude != 0 || s.Longitude != 0 {
		if m, err := g.fetchSoilMoisture(ctx, s.Latitude, s.Longitude); err == nil && m >= 0 {
			seed = m
		}
	}

	g.moisture = clamp01(seed + g.pendingBoost)
	g.pendingBoost = 0
	g.last = time.Now().UTC()
	g.seeded = true
}

// Next aggiorna lo stato interno e restituisce un SensorData.
func (g *DataGenerator) Next(sensor *model.Sensor) (model.SensorData, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now().UTC()
	if !g.seeded {
		// Se non è stato chiamato esplicitamente SeedFromSoilGrids, seed di default al primo uso.
		g.moisture = clamp01(defaultSeed + g.pendingBoost)
		g.pendingBoost = 0
		g.last = now
		g.seeded = true
	}

	dtMin := now.Sub(g.last).Minutes()
	if dtMin < 0 {
		dtMin = 0
	}

	switch sensor.State {
	case model.StateOn:
		g.moisture = clamp01(g.moisture + gainPerMin*dtMin)
	default: // OFF
		g.moisture = clamp01(g.moisture - g.decayPerMin*dtMin)
	}
	g.last = now

	return model.SensorData{
		FieldID:    sensor.FieldID,
		SensorID:   sensor.ID,
		Moisture:   int(math.Round(g.moisture * 100)), // percentuale 0..100
		Aggregated: false,
		Timestamp:  now,
	}, nil
}

// ApplyIrrigation permette di accumulare un boost pre-seed.
// (Se già seedato: l'aumento avviene progressivamente mentre lo stato è ON.)
func (g *DataGenerator) ApplyIrrigation(d time.Duration) {
	if g == nil || d <= 0 {
		return
	}
	inc := gainPerMin * d.Minutes() // in [0..1]
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.seeded {
		g.pendingBoost += inc
	}
}

// ===== Helpers =====

func (g *DataGenerator) fetchSoilMoisture(ctx context.Context, lat, lon float64) (float64, error) {
	url := fmt.Sprintf(soilGridsURL, lat, lon)
	var lastErr error

	attemptOnce := func() (val float64, retry bool, err error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return -1, true, err
		}
		req.Header.Set("User-Agent", "sdcc-sensor-simulator/1.0")

		resp, err := g.httpClient.Do(req)
		if err != nil {
			return -1, true, err
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		closeErr := resp.Body.Close()

		// preferiamo il readErr; il closeErr viene propagato se non c'è altro
		if readErr != nil {
			if closeErr != nil {
				// conserva il readErr come principale, ma segnala che c'è stato anche closeErr
				return -1, true, fmt.Errorf("%w; body close error: %v", readErr, closeErr)
			}
			return -1, true, readErr
		}

		switch {
		case resp.StatusCode == http.StatusOK:
			var parsed any
			if err := json.Unmarshal(body, &parsed); err != nil {
				if closeErr != nil {
					return -1, true, fmt.Errorf("%w; body close error: %v", err, closeErr)
				}
				return -1, true, err
			}
			if m := extractMoistureHeuristic(parsed); m >= 0 {
				if closeErr != nil {
					return -1, false, closeErr
				}
				return normalizeWV(m), false, nil
			}
			if closeErr != nil {
				return -1, true, fmt.Errorf("soilgrids: moisture field not found; body close error: %v", closeErr)
			}
			return -1, true, errors.New("soilgrids: moisture field not found")

		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			// retryable
			if closeErr != nil {
				return -1, true, fmt.Errorf("soilgrids HTTP %d; body close error: %v", resp.StatusCode, closeErr)
			}
			return -1, true, fmt.Errorf("soilgrids HTTP %d", resp.StatusCode)

		default:
			// non-retryable → ritorno immediato
			if closeErr != nil {
				return -1, false, closeErr
			}
			return -1, false, fmt.Errorf("soilgrids HTTP %d: %s", resp.StatusCode, string(body))
		}
	}

	for attempt := 0; attempt < 2; attempt++ {
		if val, retry, err := attemptOnce(); err == nil {
			return val, nil
		} else {
			lastErr = err
			if !retry {
				return -1, lastErr
			}
		}
		if attempt == 0 {
			time.Sleep(time.Duration(rand.Intn(400)+600) * time.Millisecond)
		}
	}
	return -1, lastErr
}

// Prova a trovare un valore numerico di moisture in strutture comuni della risposta.
func extractMoistureHeuristic(v any) float64 {
	// Pattern tipici (non garantiti):
	//  - {"properties":{"layers":[{"name":"wv0010","depths":[{"values":{"Q0.5":0.27}}]}]}}
	//  - {"features":[{"properties":{"layers":[... come sopra ...]}}]}
	switch m := v.(type) {
	case map[string]any:
		// features path
		if feats, ok := m["features"].([]any); ok && len(feats) > 0 {
			if f0, ok := feats[0].(map[string]any); ok {
				if p, ok := f0["properties"].(map[string]any); ok {
					if x := extractFromProperties(p); x >= 0 {
						return x
					}
				}
			}
		}
		// direct properties
		if p, ok := m["properties"].(map[string]any); ok {
			if x := extractFromProperties(p); x >= 0 {
				return x
			}
		}
	}
	return -1
}

func extractFromProperties(p map[string]any) float64 {
	layersVal, ok := p["layers"]
	if !ok {
		return -1
	}
	layers, ok := layersVal.([]any)
	if !ok || len(layers) == 0 {
		return -1
	}
	// primo layer
	l0, ok := layers[0].(map[string]any)
	if !ok {
		return -1
	}
	// depths
	depthsVal, ok := l0["depths"]
	if !ok {
		return -1
	}
	depths, ok := depthsVal.([]any)
	if !ok || len(depths) == 0 {
		return -1
	}
	d0, ok := depths[0].(map[string]any)
	if !ok {
		return -1
	}
	vals, ok := d0["values"].(map[string]any)
	if !ok {
		return -1
	}

	// Preferenze e robustezza su tipi numerici:
	for _, k := range []string{"Q0.5", "mean", "Q0.95", "Q0.05", "value", "MED"} {
		raw, ok := vals[k]
		if !ok || raw == nil {
			continue
		}
		switch t := raw.(type) {
		case float64:
			return t
		case json.Number:
			if f, err := t.Float64(); err == nil {
				return f
			}
		case int:
			return float64(t)
		case int64:
			return float64(t)
		}
	}
	return -1
}

// normalizeWV porta i valori SoilGrids "wv****" nello stesso dominio del simulatore (0..1).
// Molti layer SoilGrids sono espressi come interi in millesimi di m3/m3 (es. 420 => 0.420).
func normalizeWV(x float64) float64 {
	// euristica robusta: qualunque valore > 1.5 è quasi sicuramente "compresso" in millesimi
	if x > 1.5 {
		x = x / 1000.0
	}
	if x < 0 {
		x = 0
	}
	if x > 1 {
		x = 1
	}
	return x
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
