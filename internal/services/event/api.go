package event

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

type Irrigation struct {
	SensorID string `json:"sensor_id"`
	Amount   int    `json:"amount"` // dose in mm (arrotondata)
	Time     string `json:"time"`   // RFC3339
}

func NewIrrigationsHandler(client influxdb2.Client, org, bucket string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := 20
		if s := r.URL.Query().Get("limit"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}

		q := `
from(bucket:"` + bucket + `")
|> range(start:-7d)
|> filter(fn:(r)=> r._measurement=="system_event" and r.event_type=="irrigation.decision" and r._field=="dose_mm")
|> keep(columns:["sensor_id","_time","_value"])
|> sort(columns:["_time"], desc:true)
|> limit(n:` + strconv.Itoa(limit) + `)
`
		api := client.QueryAPI(org)
		res, err := api.Query(r.Context(), q)
		if err != nil {
			http.Error(w, "query error", http.StatusBadGateway)
			return
		}
		defer res.Close()

		out := make([]Irrigation, 0, limit)
		for res.Next() {
			sid, _ := res.Record().ValueByKey("sensor_id").(string)
			val, _ := res.Record().Value().(float64)
			ts := res.Record().Time()
			out = append(out, Irrigation{
				SensorID: sid,
				Amount:   int(val + 0.5), // arrotonda
				Time:     ts.UTC().Format(time.RFC3339),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
}
