package entities

// Field represents a tract of land growing a particular crop,
// and contains one or more sensors.
type Field struct {
	ID       string   `json:"id"`        // unique field identifier
	CropType string   `json:"crop_type"` // e.g. "corn", "wheat"
	Sensors  []Sensor `json:"sensors"`   // all sensors in this field
}

func (f *Field) GetSensor(sensorID string) *Sensor {
	for i := range f.Sensors {
		if f.Sensors[i].ID == sensorID {
			return &f.Sensors[i]
		}
	}
	return nil
}
