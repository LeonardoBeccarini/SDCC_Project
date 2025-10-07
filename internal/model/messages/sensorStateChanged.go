package messages

import (
	"time"

	"github.com/LeonardoBeccarini/sdcc_project/internal/model/entities"
)

// StateChangeEvent per cambio di stato del sensore
type StateChangeEvent struct {
	FieldID   string               `json:"field_id"`
	SensorID  string               `json:"sensor_id"`
	NewState  entities.SensorState `json:"new_state"`
	Duration  time.Duration        `json:"duration"`
	Timestamp time.Time            `json:"timestamp"`
}
