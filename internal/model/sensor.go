package model

import "time"

// SensorState indicates whether the irrigation valve is on or off.
type SensorState string

const (
	StateOff SensorState = "off"
	StateOn  SensorState = "on"
)

// Sensor represents a single device in the field.
type Sensor struct {
	ID       string      `json:"id"`       // unique sensor identifier
	State    SensorState `json:"state"`    // irrigation on/off
	FlowRate float64     `json:"capacity"` // lt/min
}

// func to compute sensor uptime to satisfy irrigation demand
func (s Sensor) ComputeDuration(waterQuantity float64) time.Duration {
	if s.FlowRate <= 0 {
		return 0
	}
	seconds := waterQuantity / s.FlowRate
	return time.Duration(seconds * float64(time.Second))
}
