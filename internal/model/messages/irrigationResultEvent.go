package messages

import "time"

// IrrigationResultEvent è pubblicato dal device-service al termine (o fallimento) dell'irrigazione.
// È allineato allo stile degli altri eventi in internal/model/messages/*.
type IrrigationResultEvent struct {
	FieldID    string    `json:"field_id"`
	SensorID   string    `json:"sensor_id"`
	TicketID   string    `json:"ticket_id"`
	DecisionID string    `json:"decision_id"`
	Status     string    `json:"status"`     // "OK" | "FAIL"
	MmApplied  float64   `json:"mm_applied"` // mm realmente erogati (>=0)
	Reason     string    `json:"reason"`     // "done" | "offline" | "timeout"
	StartedAt  time.Time `json:"started_at"` // inizio ciclo
	Timestamp  time.Time `json:"timestamp"`  // fine ciclo (ts evento)
}
