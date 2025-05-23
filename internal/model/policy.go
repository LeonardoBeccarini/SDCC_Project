// Package model internal/model/policy.go
package model

// Policy holds your soil‐moisture thresholds.
type Policy struct {
	MoistureThreshold int     `json:"moisture_threshold"`
	WaterQuantity     float64 `json:"water_quantity"` // liters of water for irrigation when soil moisture drops below threshold
}
