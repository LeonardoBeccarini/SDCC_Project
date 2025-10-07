package messages

import (
	"time"
)

// mantiene sia i dati in tempo reale che quelli aggregati.
type SensorData struct {
	FieldID    string    `json:"field_id"`
	SensorID   string    `json:"sensor_id"`
	Moisture   int       `json:"moisture"`
	Aggregated bool      `json:"aggregated"`
	Timestamp  time.Time `json:"timestamp"`
}
