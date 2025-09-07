package entities

// SensorState indicates whether the irrigation valve is on or off.
type SensorState string

const (
	StateOff SensorState = "off"
	StateOn  SensorState = "on"
)

// Sensor represents a single device in the field.
type Sensor struct {
	FieldID   string      `json:"field_id"`
	ID        string      `json:"id"` // unique sensor identifier
	Longitude float64     `json:"longitude"`
	Latitude  float64     `json:"latitude"`
	MaxDepth  int         `json:"max_depth"`
	State     SensorState `json:"state"`               // irrigation on/off
	FlowLpm   float64     `json:"flow_rate,omitempty"` // portata impianto [litri/min]
	AreaM2    float64     `json:"area_m2,omitempty"`   // superficie irrigata [m^2]
}

// MMPerMinute calcola i mm/min (L/min / m^2). Se dati insufficienti → 0.
func (s Sensor) MMPerMinute() float64 {
	if s.AreaM2 <= 0 || s.FlowLpm <= 0 {
		return 0
	}
	// 1 mm su 1 m^2 = 1 L ⇒ L/min / m^2 = mm/min
	return s.FlowLpm / s.AreaM2
}
