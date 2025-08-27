package event

import (
	"encoding/json"
	"net/http"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

type healthHandler struct {
	mqtt   mqtt.Client
	influx influxdb2.Client
	writer *Writer
}

func NewHealthHandler(m mqtt.Client, i influxdb2.Client, w *Writer) http.Handler {
	return &healthHandler{mqtt: m, influx: i, writer: w}
}

func (h *healthHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) { // r non usato -> sostituito con _
	type status struct {
		Status          string  `json:"status"`
		MQTTConnected   bool    `json:"mqtt_connected"`
		InfluxOK        bool    `json:"influx_ok"`
		LastWriteErrorS float64 `json:"last_write_error_age_sec"`
	}
	st := status{
		MQTTConnected:   h.mqtt != nil && h.mqtt.IsConnectionOpen(),
		InfluxOK:        h.influx != nil, // esistenza client (check leggero)
		LastWriteErrorS: h.writer.LastErrorAge().Seconds(),
	}

	// ok se deps ok e nessun errore recente di scrittura
	if st.MQTTConnected && st.InfluxOK && h.writer.LastErrorAge() > 30*time.Second {
		st.Status = "ok"
	} else if st.MQTTConnected || st.InfluxOK {
		st.Status = "degraded"
	} else {
		st.Status = "down"
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

// Handler /readyz: 200 solo se tutte le dipendenze sono ok.
type readyHandler struct {
	mqtt     mqtt.Client
	influx   influxdb2.Client
	writer   *Writer
	minError time.Duration
}

func NewReadyHandler(m mqtt.Client, i influxdb2.Client, w *Writer, minOkErrorAge time.Duration) http.Handler {
	return &readyHandler{mqtt: m, influx: i, writer: w, minError: minOkErrorAge}
}

func (h *readyHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) { // r non usato -> _
	ready := h.mqtt != nil && h.mqtt.IsConnectionOpen() && h.influx != nil && h.writer.LastErrorAge() > h.minError
	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	w.Header().Set("Content-Type", "application/json")
	type resp struct {
		Ready bool `json:"ready"`
	}
	_ = json.NewEncoder(w).Encode(resp{Ready: ready})
}
