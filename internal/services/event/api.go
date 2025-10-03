package event

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

// Payload esposta al gateway
type Irrigation struct {
	SensorID string  `json:"sensor_id,omitempty"`
	Amount   float64 `json:"amount"` // mm (mappa da dose_mm)
	Time     string  `json:"time"`   // RFC3339
}

type irrQueryParams struct {
	Minutes   int
	Limit     int
	TimeoutMS int
}

func parseIrr(r *http.Request, defMin, defLim, defTOms int) irrQueryParams {
	q := r.URL.Query()
	get := func(k string, def, min, max int) int {
		if v := strings.TrimSpace(q.Get(k)); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				if n < min {
					return min
				}
				if max > 0 && n > max {
					return max
				}
				return n
			}
		}
		return def
	}
	return irrQueryParams{
		Minutes:   get("minutes", defMin, 1, 7*24*60),
		Limit:     get("limit", defLim, 1, 500),
		TimeoutMS: get("timeout_ms", defTOms, 200, 5000),
	}
}

func buildFlux(bucket string, minutes, limit int) string {
	return fmt.Sprintf(`
from(bucket: %q)
  |> range(start: -%dm)
  |> filter(fn: (r) => r._measurement == "system_event" and r.event_type == "irrigation.result")
  |> filter(fn: (r) => r._field == "mm_applied")
  |> keep(columns: ["_time","_value","sensor_id"])
  |> sort(columns: ["_time"], desc: true)
  |> limit(n:%d)
`, bucket, minutes, limit)
}

func runIrr(w http.ResponseWriter, r *http.Request, influx influxdb2.Client, org, bucket string, defMin, defLim int) {
	p := parseIrr(r, defMin, defLim, 2000)

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(p.TimeoutMS)*time.Millisecond)
	defer cancel()

	api := influx.QueryAPI(org)
	res, err := api.Query(ctx, buildFlux(bucket, p.Minutes, p.Limit))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Error", "influx-query-error")
		_, _ = w.Write([]byte("[]"))
		return
	}
	defer func() {
		if cerr := res.Close(); cerr != nil {
		}
	}()

	out := make([]Irrigation, 0, p.Limit)
	for res.Next() {
		rec := res.Record()

		// dose_mm -> amount
		var amount float64
		switch v := rec.Value().(type) {
		case float64:
			amount = v
		case int64:
			amount = float64(v)
		case int:
			amount = float64(v)
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				amount = f
			}
		}

		var sensorID string
		if v := rec.ValueByKey("sensor_id"); v != nil {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				sensorID = s
			}
		}

		out = append(out, Irrigation{
			SensorID: sensorID,
			Amount:   amount,
			Time:     rec.Time().UTC().Format(time.RFC3339),
		})
	}
	if res.Err() != nil {
		w.Header().Set("X-Error", "influx-iter-error")
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// === HANDLER PUBBLICO UNIFICATO ===
// GET /events/irrigation/latest?limit=20[&minutes=1440]
func NewIrrigationLatestHandler(influx influxdb2.Client, org, bucket string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runIrr(w, r, influx, org, bucket, 1440, 20)
	})
}
