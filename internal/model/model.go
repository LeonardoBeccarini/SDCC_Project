package model

import (
	"github.com/LeonardoBeccarini/sdcc_project/internal/model/entities"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model/messages"
)

// Alias per esporre tipi comuni ai servizi

type (
	SensorData       = messages.SensorData
	StateChangeEvent = messages.StateChangeEvent
	Field            = entities.Field
	Policy           = entities.Policy
	Sensor           = entities.Sensor
)

const (
	StateOn  = entities.StateOn
	StateOff = entities.StateOff
)
