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
