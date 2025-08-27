package event

import (
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

// EventToPoint normalizza CommonEvent in un *write.Point per InfluxDB.
func EventToPoint(evt CommonEvent) *write.Point {
	// Tag (solo stringhe)
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

	// Fields: prendi quelli dell'evento (se nil, crea una mappa vuota)
	fields := map[string]interface{}{}
	if evt.Fields != nil {
		for k, v := range evt.Fields {
			fields[k] = v
		}
	}

	// per sicurezza, aggiungi un contatore monotono per avere almeno un field
	if _, ok := fields["count"]; !ok {
		fields["count"] = int64(1)
	}

	// Misura unica "system_event"
	return influxdb2.NewPoint("system_event", tags, fields, evt.Timestamp)
}
