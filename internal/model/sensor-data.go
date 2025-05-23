package model

import "time"

type SensorData struct {
	SensorID string `json:"sensor_id"`
	FieldID  string `json:"field_id"`
	Moisture int    `json:"moisture"`
	//Temperature float64   `json:"temperature"`
	//WindSpeed   float64   `json:"wind_speed"`
	Timestamp time.Time `json:"timestamp"`
}
