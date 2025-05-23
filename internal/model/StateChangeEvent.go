package model

import "time"

// StateChangeEvent is emitted when a sensor needs a new irrigation state.
type StateChangeEvent struct {
	FieldID   string        `json:"field_id"`
	SensorID  string        `json:"sensor_id"`
	NewState  SensorState   `json:"new_state"`
	Duration  time.Duration `json:"duration"`
	Timestamp time.Time     `json:"timestamp"`
}
