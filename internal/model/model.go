package model

import (
	// re-export dei tipi “sorgente”
	"github.com/LeonardoBeccarini/sdcc_project/internal/model/entities"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model/messages"
)

//
// Alias di tipi da usare nei servizi
//

// --- Telemetria e messaggi ---
type (
	SensorData              = messages.SensorData              // misura suolo (raw/aggregata)
	StateChangeEvent        = messages.StateChangeEvent        // evento cambio stato sensore
	IrrigationDecisionEvent = messages.IrrigationDecisionEvent // evento decisione irrigazione
	IrrigationResultEvent   = messages.IrrigationResultEvent   //evento esito irrigazione

	// Entità/parametri (già presenti)
	Sensor           = entities.Sensor
	Field            = entities.Field
	State            = entities.SensorState // stato del dispositivo (se definito in entities)
	IrrigationPolicy = entities.IrrigationPolicy
)

// --- Costanti di stato (se presenti in entities) ---
const (
	StateOn  = entities.StateOn
	StateOff = entities.StateOff
)

// Nota:
// - Usiamo *type alias* (es. `type X = pkg.Y`), non nuove definizioni: i tipi restano
//   identici a quelli originali, con lo stesso set di metodi e tag JSON.
// - Per aggiungere nuovi tipi in futuro, basta estendere le sezioni sopra.
// - Evita che `entities` o `messages` importino `internal/model`, altrimenti crei
//   un ciclo di import.
