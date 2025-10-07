package entities

// IrrigationPolicy utilizzata dal controller
type IrrigationPolicy struct {
	FieldID string                `json:"field_id"`
	Crop    string                `json:"crop"`
	Stage   Stage                 `json:"stage"`
	Soil    SoilProfile           `json:"soil"`
	Stages  map[Stage]StageParams `json:"stages"`
}
