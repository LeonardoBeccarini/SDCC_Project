package messages

import "time"

// IrrigationDecisionEvent is published by IrrigationController to record WHY/WHAT was applied.
type IrrigationDecisionEvent struct {
	FieldID        string    `json:"field_id"`
	SensorID       string    `json:"sensor_id"`
	Stage          string    `json:"stage"`
	DrPct          float64   `json:"dr_pct"`
	SMT            float64   `json:"smt_pct"`
	DoseMM         float64   `json:"dose_mm"`
	RemainingToday float64   `json:"remaining_today_mm"`
	Timestamp      time.Time `json:"timestamp"`
}
