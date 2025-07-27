package messages

import (
	"time"
)

// this will hold both real-time and aggregated data.
type SensorData struct {
	FieldId    string    `json:"field_id"`
	SensorID   string    `json:"sensor_id"`
	Moisture   int       `json:"moisture"`
	Aggregated bool      `json:"aggregated"`
	Timestamp  time.Time `json:"timestamp"`
}
