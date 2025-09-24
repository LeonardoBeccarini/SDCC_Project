package event

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	msg "github.com/LeonardoBeccarini/sdcc_project/internal/model/messages"
)

type CommonEvent struct {
	EventType     string // irrigation.decision | device.state_change | irrigation.result
	SourceService string // irrigation-controller | device-service | ...
	FieldID       string
	SensorID      string
	Severity      string // info|warning|error
	Fields        map[string]interface{}
	Timestamp     time.Time
}

// MQTTHandler trasforma messaggi MQTT in CommonEvent e li passa a sink (Influx).
type MQTTHandler struct{ sink func(CommonEvent) }

func NewMQTTHandler(sink func(CommonEvent)) *MQTTHandler { return &MQTTHandler{sink: sink} }

func (h *MQTTHandler) Handle(_ string, m mqtt.Message) error {
	topic := m.Topic()
	payload := m.Payload()

	var (
		evt CommonEvent
		err error
	)
	switch {
	case strings.HasPrefix(topic, "event/irrigationDecision/"):
		evt, err = decodeDecision(topic, payload)
	case strings.HasPrefix(topic, "event/StateChange/"):
		evt, err = decodeStateChange(topic, payload)
	case strings.HasPrefix(topic, "event/irrigationResult/"):
		evt, err = decodeIrrigationResult(topic, payload)
	default:
		return nil // ignora altri topic
	}
	if err != nil {
		return err
	}
	if h.sink != nil {
		h.sink(evt)
	}
	return nil
}

func decodeDecision(topic string, payload []byte) (CommonEvent, error) {
	var d msg.IrrigationDecisionEvent
	if err := json.Unmarshal(payload, &d); err != nil {
		return CommonEvent{}, err
	}
	fieldID, sensorID := pickIDs(topic, d.FieldID, d.SensorID, "event/irrigationDecision/")
	if fieldID == "" || sensorID == "" {
		return CommonEvent{}, errors.New("decision: missing field/sensor")
	}
	return CommonEvent{
		EventType:     "irrigation.decision",
		SourceService: "irrigation-controller",
		FieldID:       fieldID,
		SensorID:      sensorID,
		Severity:      "info",
		Fields: map[string]interface{}{
			"dose_mm":         d.DoseMM,
			"remaining_today": d.RemainingToday,
			"dr_pct":          d.DrPct,
			"smt_pct":         d.SMT,
		},
		Timestamp: d.Timestamp,
	}, nil
}

func decodeStateChange(topic string, payload []byte) (CommonEvent, error) {
	var s msg.StateChangeEvent
	if err := json.Unmarshal(payload, &s); err != nil {
		return CommonEvent{}, err
	}
	fieldID, sensorID := pickIDs(topic, s.FieldID, s.SensorID, "event/StateChange/")
	if fieldID == "" || sensorID == "" {
		return CommonEvent{}, errors.New("stateChange: missing field/sensor")
	}
	return CommonEvent{
		EventType:     "device.state_change",
		SourceService: "device-service",
		FieldID:       fieldID,
		SensorID:      sensorID,
		Severity:      "info",
		Fields: map[string]interface{}{
			"new_state": s.NewState,
			"duration":  s.Duration.Seconds(),
		},
		Timestamp: s.Timestamp,
	}, nil
}

func decodeIrrigationResult(topic string, payload []byte) (CommonEvent, error) {
	var r msg.IrrigationResultEvent
	if err := json.Unmarshal(payload, &r); err != nil {
		return CommonEvent{}, err
	}
	fieldID, sensorID := pickIDs(topic, r.FieldID, r.SensorID, "event/irrigationResult/")
	if fieldID == "" || sensorID == "" {
		return CommonEvent{}, errors.New("result: missing field/sensor")
	}
	sev := "info"
	if strings.EqualFold(r.Status, "FAIL") {
		sev = "warning"
	}
	return CommonEvent{
		EventType:     "irrigation.result",
		SourceService: "device-service",
		FieldID:       fieldID,
		SensorID:      sensorID,
		Severity:      sev,
		Fields: map[string]interface{}{
			"status":     r.Status,
			"mm_applied": r.MmApplied,
			"reason":     r.Reason,
		},
		Timestamp: r.Timestamp,
	}, nil
}

// pickIDs usa payload, oppure topic "prefix/{field}/{sensor}".
func pickIDs(topic, fieldID, sensorID, prefix string) (string, string) {
	if strings.TrimSpace(fieldID) != "" && strings.TrimSpace(sensorID) != "" {
		return fieldID, sensorID
	}
	suffix := strings.TrimPrefix(topic, prefix)
	parts := strings.Split(suffix, "/")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	return fieldID, sensorID
}
