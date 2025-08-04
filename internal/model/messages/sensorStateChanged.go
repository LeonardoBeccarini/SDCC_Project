package messages

import (
	"github.com/LeonardoBeccarini/sdcc_project/internal/model/entities"
	"time"
)

const (
	StateOn  = "ON"
	StateOff = "OFF"
)

// StateChangeEvent is emitted when a sensor needs a new irrigation state.
type StateChangeEvent struct {
	FieldID   string               `json:"field_id"`
	SensorID  string               `json:"sensor_id"`
	NewState  entities.SensorState `json:"new_state"`
	Duration  time.Duration        `json:"duration"`
	Timestamp time.Time            `json:"timestamp"`
}
