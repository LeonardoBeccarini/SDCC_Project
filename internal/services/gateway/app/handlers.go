package app

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"sort"
)

func (g *Gateway) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), g.cfg.HTTPTimeout)
	defer cancel()

	type res struct {
		key string
		val any
	}
	ch := make(chan res, 2)

	// Fetch in parallelo
	go func() {
		var latest []PersistenceLatest
		_ = g.persistence.GetJSON(ctx, &latest)
		ch <- res{"sensors", latest}
	}()
	go func() {
		var irr []Irrigation
		_ = g.events.GetJSON(ctx, &irr)
		ch <- res{"irrigations", irr}
	}()

	data := DashboardData{
		Sensors:     []PersistenceLatest{},
		Irrigations: []Irrigation{},
		Stats:       map[string]float64{},
	}

	for i := 0; i < 2; i++ {
		rv := <-ch
		switch rv.key {
		case "sensors":
			if s, ok := rv.val.([]PersistenceLatest); ok {
				data.Sensors = s
			}
		case "irrigations":
			if ir, ok := rv.val.([]Irrigation); ok {
				data.Irrigations = ir
			}
		}
	}

	// Ordine sensori e statistiche per la UI
	sort.Slice(data.Sensors, func(i, j int) bool { return data.Sensors[i].SensorID < data.Sensors[j].SensorID })
	if n := len(data.Sensors); n > 0 {
		var sum, minv, maxv float64
		minv = math.MaxFloat64
		for _, s := range data.Sensors {
			v := float64(s.Moisture)
			sum += v
			if v < minv {
				minv = v
			}
			if v > maxv {
				maxv = v
			}
		}
		data.Stats["mean"] = math.Round(sum / float64(n)) // intero arrotondato
		data.Stats["min"] = minv
		data.Stats["max"] = maxv
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}
