package app

import (
	"encoding/json"
	"math"
	"strconv"
)

// ---------- Upstream payloads ----------

type PersistenceLatest struct {
	SensorID   string `json:"sensor_id"`
	Moisture   int    `json:"moisture"`
	Aggregated bool   `json:"aggregated"`
	Time       string `json:"time"` // RFC3339
}

func (p *PersistenceLatest) UnmarshalJSON(b []byte) error {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	if v, ok := m["sensor_id"].(string); ok {
		p.SensorID = v
	}
	// aggregated: default false se mancante
	if v, ok := m["aggregated"].(bool); ok {
		p.Aggregated = v
	}
	// time / timestamp
	if t, ok := m["timestamp"].(string); ok && t != "" {
		p.Time = t
	} else if t, ok := m["time"].(string); ok && t != "" {
		p.Time = t
	}
	// moisture come numero o stringa
	if mv, ok := m["moisture"]; ok {
		switch x := mv.(type) {
		case float64:
			p.Moisture = int(math.Round(x))
		case string:
			if f, err := strconv.ParseFloat(x, 64); err == nil {
				p.Moisture = int(math.Round(f))
			}
		case bool:
			if x {
				p.Moisture = 1
			}
		default:
			// se arriva come int (raro con encoding/json default) lo intercettiamo via stringa/f64
		}
	}
	return nil
}

type Irrigation struct {
	SensorID string `json:"sensor_id"`
	Amount   int    `json:"amount"` // accettiamo anche "amount_mm"
	Time     string `json:"time"`   // RFC3339
}

func (i *Irrigation) UnmarshalJSON(b []byte) error {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	if v, ok := m["sensor_id"].(string); ok {
		i.SensorID = v
	}
	if t, ok := m["time"].(string); ok && t != "" {
		i.Time = t
	} else if t, ok := m["timestamp"].(string); ok && t != "" {
		i.Time = t
	}

	getNum := func(key string) (int, bool) {
		if mv, ok := m[key]; ok {
			switch x := mv.(type) {
			case float64:
				return int(math.Round(x)), true
			case string:
				if f, err := strconv.ParseFloat(x, 64); err == nil {
					return int(math.Round(f)), true
				}
			case bool:
				if x {
					return 1, true
				}
			}
		}
		return 0, false
	}

	if n, ok := getNum("amount"); ok {
		i.Amount = n
	} else if n, ok := getNum("amount_mm"); ok {
		i.Amount = n
	}
	return nil
}

type DashboardData struct {
	Sensors     []PersistenceLatest `json:"sensors"`
	Irrigations []Irrigation        `json:"irrigations"`
	Stats       map[string]float64  `json:"stats"`
}
