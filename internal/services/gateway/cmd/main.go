package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
)

/************* MODELS *************/
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

/************* INFLUX CLIENT *************/
type Influx struct {
	client influxdb2.Client
	query  api.QueryAPI
	cfg    Config
}

func newInflux(cfg Config) *Influx {
	c := influxdb2.NewClient(cfg.InfluxURL, cfg.InfluxToken)
	return &Influx{client: c, query: c.QueryAPI(cfg.InfluxOrg), cfg: cfg}
}
func (ix *Influx) Close() { ix.client.Close() }

func (ix *Influx) RecentIrrigations(ctx context.Context, n int) ([]Irrigation, error) {
	flux := `
from(bucket:"` + ix.cfg.InfluxBucket + `")
|> range(start:-7d)
|> filter(fn:(r)=> r._measurement=="system_event" and r.event_type=="irrigation.decision")
|> sort(columns:["_time"], desc:true)
|> limit(n:` + strconv.Itoa(n) + `)
|> keep(columns: ["_time","sensor_id","_value","dose_mm"])
`
	res, err := ix.query.Query(ctx, flux)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	out := make([]Irrigation, 0, n)
	for res.Next() {
		amt := 0
		if v, ok := res.Record().ValueByKey("dose_mm").(float64); ok {
			amt = int(v + 0.5)
		} else if v, ok := res.Record().Value().(float64); ok {
			amt = int(v + 0.5)
		}
		sid, _ := res.Record().ValueByKey("sensor_id").(string)
		out = append(out, Irrigation{
			SensorID: sid,
			Amount:   amt,
			Time:     res.Record().Time().Format("15:04"),
		})
	}
	return out, res.Err()
}

func (ix *Influx) LatestSensors(ctx context.Context, metric string) ([]Sensor, error) {
	flux := `
from(bucket:"` + ix.cfg.InfluxBucket + `")
|> range(start:-6h)
|> filter(fn:(r)=> r._measurement=="sensors" and r.metric=="` + metric + `")
|> group(columns:["sensor_id"])
|> last()
|> keep(columns:["sensor_id","_value"])
`
	res, err := ix.query.Query(ctx, flux)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	var out []Sensor
	for res.Next() {
		id, _ := res.Record().ValueByKey("sensor_id").(string)
		val, _ := res.Record().Value().(float64)
		out = append(out, Sensor{ID: id, Value: val})
	}
	return out, res.Err()
}

func (ix *Influx) LatestStatuses(ctx context.Context) (map[string]string, error) {
	flux := `
from(bucket:"` + ix.cfg.InfluxBucket + `")
|> range(start:-7d)
|> filter(fn:(r)=> r._measurement=="system_event" and r.event_type=="device.state_change")
|> group(columns:["sensor_id"])
|> last()
|> keep(columns:["sensor_id","status","_value"])
`
	res, err := ix.query.Query(ctx, flux)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	out := map[string]string{}
	for res.Next() {
		id, _ := res.Record().ValueByKey("sensor_id").(string)
		st, _ := res.Record().ValueByKey("status").(string)
		if st == "" {
			if v, ok := res.Record().Value().(float64); ok && v > 0 {
				st = "on"
			} else {
				st = "off"
			}
		}
		out[id] = st
	}
	return out, res.Err()
}

func (ix *Influx) MetricStats(ctx context.Context, metric string) (Stats, error) {
	st := Stats{}
	// mean
	fluxMean := `
from(bucket:"` + ix.cfg.InfluxBucket + `")
|> range(start:-6h)
|> filter(fn:(r)=> r._measurement=="sensors" and r.metric=="` + metric + `")
|> mean()
|> keep(columns:["_value"])
|> last()
`
	if res, err := ix.query.Query(ctx, fluxMean); err == nil {
		for res.Next() {
			if v, ok := res.Record().Value().(float64); ok {
				st.Mean = v
			}
		}
		res.Close()
	}
	// max
	fluxMax := `
from(bucket:"` + ix.cfg.InfluxBucket + `")
|> range(start:-6h)
|> filter(fn:(r)=> r._measurement=="sensors" and r.metric=="` + metric + `")
|> max()
|> keep(columns:["_value"])
|> last()
`
	if res, err := ix.query.Query(ctx, fluxMax); err == nil {
		for res.Next() {
			if v, ok := res.Record().Value().(float64); ok {
				st.Max = v
			}
		}
		res.Close()
	}
	// min
	fluxMin := `
from(bucket:"` + ix.cfg.InfluxBucket + `")
|> range(start:-6h)
|> filter(fn:(r)=> r._measurement=="sensors" and r.metric=="` + metric + `")
|> min()
|> keep(columns:["_value"])
|> last()
`
	if res, err := ix.query.Query(ctx, fluxMin); err == nil {
		for res.Next() {
			if v, ok := res.Record().Value().(float64); ok {
				st.Min = v
			}
		}
		res.Close()
	}
	return st, nil
}

/************* HELPERS *************/
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

/************* HTTP FALLBACK CLIENT *************/
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
		return fmt.Errorf("upstream %s status=%d", url, res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func (u *Upstream) GetStatuses(ctx context.Context, base string) map[string]string {
	type resp map[string]string
	var out resp
	_ = u.getJSON(ctx, base+"/sensors/status", &out)
	return out
}
func (u *Upstream) GetValues(ctx context.Context, base string) []Sensor {
	var out []Sensor
	_ = u.getJSON(ctx, base+"/data/latest", &out)
	return out
}
func (u *Upstream) GetEvents(ctx context.Context, base string) []Irrigation {
	var out []Irrigation
	_ = u.getJSON(ctx, base+"/irrigations/recent", &out)
	return out
}
func (u *Upstream) GetStats(ctx context.Context, base string) Stats {
	var out Stats
	_ = u.getJSON(ctx, base+"/analytics/stats", &out)
	return out
}

/************* MAIN (HTTP) *************/
func main() {
	cfg := loadConfig()
	ix := newInflux(cfg)
	defer ix.Close()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})

	http.HandleFunc("/dashboard/data", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.TimeoutMs)*time.Millisecond)
		defer cancel()

		up := NewUpstream(cfg.TimeoutMs)

		// 1) tentativo via Influx (primario)
		statuses, errS := ix.LatestStatuses(ctx)
		events, errE := ix.RecentIrrigations(ctx, 20)
		stats, errT := ix.MetricStats(ctx, cfg.MetricName)
		values, errV := ix.LatestSensors(ctx, cfg.MetricName)

		okInflux := (errS == nil && errE == nil && errT == nil && errV == nil)

		var resp Payload
		if okInflux {
			resp = Payload{
				Sensors:     applyStatus(values, statuses),
				Irrigations: events,
				Stats:       stats,
			}
		} else {
			// 2) Fallback ai microservizi REST
			statuses2 := up.GetStatuses(ctx, cfg.DeviceURL)
			values2 := up.GetValues(ctx, cfg.PersistenceURL)
			events2 := up.GetEvents(ctx, cfg.EventURL)
			stats2 := up.GetStats(ctx, cfg.AnalyticsURL)

			if len(values2) > 0 || len(events2) > 0 || (stats2 != (Stats{})) || len(statuses2) > 0 {
				resp = Payload{
					Sensors:     applyStatus(values2, statuses2),
					Irrigations: events2,
					Stats:       stats2,
				}
			} else {
				resp = Payload{
					Sensors:     applyStatus(values, statuses),
					Irrigations: events,
					Stats:       stats,
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		log.Printf("GET /dashboard/data [%dms] influxOK=%v sensors=%d events=%d",
			time.Since(start).Milliseconds(), okInflux, len(resp.Sensors), len(resp.Irrigations))
	})

	addr := ":" + cfg.Port
	log.Printf("Gateway listening on %s (Influx=%s bucket=%s org=%s metric=%s)",
		addr, cfg.InfluxURL, cfg.InfluxBucket, cfg.InfluxOrg, cfg.MetricName)
	log.Fatal(http.ListenAndServe(addr, nil))
}
