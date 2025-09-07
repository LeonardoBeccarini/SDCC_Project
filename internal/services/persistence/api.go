package persistence

import (
	"context"
	"encoding/json"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func NewHTTPMux(svc *Service) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })

	// GET /data/latest
	// Query params:
	//   source=auto|influx|cache   (default auto: prova Influx, fallback cache)
	//   minutes=<int>              (finestra temporale per Influx, default 1440 = 24h)
	mux.HandleFunc("/data/latest", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		source := strings.ToLower(q.Get("source"))
		if source == "" {
			source = "auto"
		}
		minutes := 60 * 24
		if s := q.Get("minutes"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				minutes = n
			}
		}

		// prefer Influx, fallback cache
		var list []model.SensorData
		var err error
		var used string

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if source == "influx" || source == "auto" {
			list, err = svc.QueryLatestFromInflux(ctx, minutes)
			if err == nil && len(list) > 0 {
				used = "influx"
			}
		}
		if used == "" { // cache path
			list = svc.LatestCache()
			used = "cache"
		}

		type outT struct {
			FieldID    string `json:"field_id"`
			SensorID   string `json:"sensor_id"`
			Moisture   int    `json:"moisture"`
			Aggregated bool   `json:"aggregated"`
			Timestamp  string `json:"timestamp"`
		}
		out := make([]outT, 0, len(list))
		for _, v := range list {
			out = append(out, outT{
				FieldID: v.FieldID, SensorID: v.SensorID, Moisture: v.Moisture,
				Aggregated: v.Aggregated, Timestamp: v.Timestamp.UTC().Format(time.RFC3339),
			})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].SensorID < out[j].SensorID })

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Data-Source", used)
		_ = json.NewEncoder(w).Encode(out)
	})

	return mux
}
