package entities

// Field rappresenta un pezzo di terra dove è coltivata una determinata coltura e contiene 1+ sensori.
type Field struct {
	ID       string   `json:"id"`
	CropType string   `json:"crop_type"`       // e.g. "corn", "wheat"
	Stage    Stage    `json:"stage,omitempty"` // ciclo sviluppo di una pianta
	Sensors  []Sensor `json:"sensors"`
}

func (f *Field) GetSensor(sensorID string) *Sensor {
	for i := range f.Sensors {
		if f.Sensors[i].ID == sensorID {
			return &f.Sensors[i]
		}
	}
	return nil
}
