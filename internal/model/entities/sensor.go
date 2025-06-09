package entities

import "time"

// SensorState indicates whether the irrigation valve is on or off.
type SensorState string

const (
	StateOff SensorState = "off"
	StateOn  SensorState = "on"
)

// Sensor represents a single device in the field.
type Sensor struct {
	FieldId   string      `json:"field_id"`
	ID        string      `json:"id"` // unique sensor identifier
	Longitude float64     `json:"longitude"`
	Latitude  float64     `json:"latitude"`
	MaxDepth  int         `json:"max_depth"`
	State     SensorState `json:"state"`    // irrigation on/off
	FlowRate  float64     `json:"capacity"` // lt/min
}

// func to compute sensor uptime to satisfy irrigation demand
func (s Sensor) ComputeDuration(waterQuantity float64) time.Duration {
	if s.FlowRate <= 0 {
		return 0
	}
	seconds := waterQuantity / s.FlowRate
	return time.Duration(seconds * float64(time.Second))
}

// ToggleIrrigation flips the irrigation state of a given sensor.
func (f *Field) ToggleIrrigation(sensorID string, on bool) {
	for i := range f.Sensors {
		if f.Sensors[i].ID == sensorID {
			if on {
				f.Sensors[i].State = StateOn
			} else {
				f.Sensors[i].State = StateOff
			}
		}
	}
}
