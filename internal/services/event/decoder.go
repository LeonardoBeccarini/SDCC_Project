package event

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	msg "github.com/LeonardoBeccarini/sdcc_project/internal/model/messages"
)

// CommonEvent is the normalized event that we persist to Influx.
type CommonEvent struct {
	EventType     string // irrigation.decision | device.state_change
	SourceService string // irrigation-controller | device | ...
	FieldID       string
	SensorID      string
	Severity      string // info|warning|error
	Fields        map[string]interface{}
	Timestamp     time.Time
}

// MQTTHandler glues MQTT messages -> CommonEvent -> sink
type MQTTHandler struct {
	sink func(CommonEvent)
}

func NewMQTTHandler(sink func(CommonEvent)) *MQTTHandler {
	return &MQTTHandler{sink: sink}
}

func (h *MQTTHandler) Handle(_ string, m mqtt.Message) error {
	topic := m.Topic()
	payload := m.Payload()

	var evt CommonEvent
	var err error

	switch {
	case strings.HasPrefix(topic, "event/irrigationDecision/"):
		evt, err = decodeDecision(topic, payload)
	case strings.HasPrefix(topic, "event/StateChange/"):
		evt, err = decodeStateChange(topic, payload)
	default:
		return nil // silently ignore unknown topics
	}
	if err != nil {
		return err
	}

	// Emit downstream
	h.sink(evt)
	return nil
}

func decodeDecision(topic string, b []byte) (CommonEvent, error) {
	var dec msg.IrrigationDecisionEvent
	if err := json.Unmarshal(b, &dec); err != nil {
		return CommonEvent{}, err
	}
	field, sensor := extractIDs(topic)

	// If timestamp not set, use now
	ts := dec.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	fields := map[string]interface{}{
		"stage":              dec.Stage,
		"dr_pct":             dec.DrPct,
		"smt_pct":            dec.SMT,
		"dose_mm":            dec.DoseMM,
		"remaining_today_mm": dec.RemainingToday,
	}

	return CommonEvent{
		EventType:     "irrigation.decision",
		SourceService: "irrigation-controller",
		FieldID:       coalesce(dec.FieldID, field),
		SensorID:      coalesce(dec.SensorID, sensor),
		Severity:      "info",
		Fields:        fields,
		Timestamp:     ts,
	}, nil
}

func decodeStateChange(topic string, b []byte) (CommonEvent, error) {
	// Accept a flexible JSON object with at least 'status' and optional 'duration' and 'reason'
	var obj map[string]interface{}
	if err := json.Unmarshal(b, &obj); err != nil {
		return CommonEvent{}, err
	}
	status, _ := obj["status"].(string)
	// Fallback per payload che usano "new_state" invece di "status"
	if strings.TrimSpace(status) == "" {
		if ns, ok := obj["new_state"].(string); ok && strings.TrimSpace(ns) != "" {
			status = ns
		}
	}
	if strings.TrimSpace(status) == "" {
		return CommonEvent{}, errors.New("missing status")
	}
	field, sensor := extractIDs(topic)

	// timestamp: allow explicit 'timestamp' (RFC3339) else now
	var ts time.Time
	if v, ok := obj["timestamp"].(string); ok {
		if p, err := time.Parse(time.RFC3339, v); err == nil {
			ts = p
		}
	}
	if ts.IsZero() {
		ts = time.Now()
	}

	fields := map[string]interface{}{
		"status": status,
	}
	if v, ok := obj["duration"].(string); ok {
		if d, err := time.ParseDuration(v); err == nil {
			fields["duration_s"] = int64(d / time.Second)
		}
	}
	if v, ok := obj["duration"].(float64); ok {
		// Go time.Duration marshalla come intero di nanosecondi; in JSON lo vedi come float64
		fields["duration_s"] = int64(v) / int64(time.Second)
	}

	if v, ok := obj["reason"].(string); ok && v != "" {
		fields["reason"] = v
	}

	return CommonEvent{
		EventType:     "device.state_change",
		SourceService: "device",
		FieldID:       field,
		SensorID:      sensor,
		Severity:      "info",
		Fields:        fields,
		Timestamp:     ts,
	}, nil
}

// extractIDs returns the last two non-empty path segments as fieldID and sensorID.
func extractIDs(topic string) (fieldID, sensorID string) {
	parts := strings.Split(topic, "/")
	// walk from end, collect two non-empty, non-wildcard segments
	var collected []string
	for i := len(parts) - 1; i >= 0 && len(collected) < 2; i-- {
		seg := strings.TrimSpace(parts[i])
		if seg == "" || seg == "#" || seg == "+" {
			continue
		}
		collected = append(collected, seg)
	}
	if len(collected) == 2 {
		sensorID = collected[0]
		fieldID = collected[1]
	}
	return
}

func coalesce(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
