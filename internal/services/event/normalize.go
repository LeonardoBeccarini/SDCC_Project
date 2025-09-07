package event

import (
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

// CommonEvent è definito in decoder.go: espone EventType, SourceService, Severity,
// FieldID, SensorID, Timestamp (time.Time), Fields map[string]any.
func EventToPoint(evt CommonEvent) *write.Point {
	tags := map[string]string{
		"event_type":     evt.EventType,
		"source_service": evt.SourceService,
		"severity":       evt.Severity,
	}
	if evt.FieldID != "" {
		tags["field_id"] = evt.FieldID
	}
	if evt.SensorID != "" {
		tags["sensor_id"] = evt.SensorID
	}

	fields := map[string]interface{}{}
	for k, v := range evt.Fields {
		fields[k] = v
	}
	if _, ok := fields["count"]; !ok {
		fields["count"] = int64(1)
	}

	return influxdb2.NewPoint("system_event", tags, fields, evt.Timestamp)
}
