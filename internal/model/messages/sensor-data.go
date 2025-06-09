package messages

import (
	"github.com/LeonardoBeccarini/sdcc_project/internal/model/entities"
	"time"
)

type SensorData struct {
	Sensor    entities.Sensor `json:"sensor"`
	Moisture  int             `json:"moisture"`
	Timestamp time.Time       `json:"timestamp"`
}
