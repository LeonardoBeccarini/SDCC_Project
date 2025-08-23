// Package model internal/model/irrigation-policy.go
package entities

// IrrigationPolicy is the advanced policy used by the controller.
type IrrigationPolicy struct {
	FieldID string                `json:"field_id"`
	Crop    string                `json:"crop"`
	Stage   Stage                 `json:"stage"` // current stage
	Soil    SoilProfile           `json:"soil"`
	Stages  map[Stage]StageParams `json:"stages"` // parameters per stage
}
