package device

import (
	"context"
	"encoding/json"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	pb "github.com/LeonardoBeccarini/sdcc_project/grpc/gen/go/irrigation"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model/messages"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
)

type PublisherFactory func(topic string) rabbitmq.IPublisher

// GrpcHandler implementa DeviceService e pubblica StateChange + Result.
type GrpcHandler struct {
	pb.UnimplementedDeviceServiceServer

	makePublisher PublisherFactory
	fields        map[string]model.Field

	// template topic per StateChange (già usato)
	topicTemplate string // es. "event/StateChange/{field}/{sensor}"

	//template Result
	resultTopicTmpl string // "event/irrigationResult/{field}/{sensor}"

	//liveness (heartbeat implicito da sensor/data)
	sensorLivenessTTL time.Duration
	offlineGrace      time.Duration
	lastSeen          sync.Map // chiave "field|sensor" -> time.Time
}

func NewGrpcHandler(factory PublisherFactory, topicTemplate string, fields map[string]model.Field) *GrpcHandler {
	return &GrpcHandler{
		makePublisher:     factory,
		fields:            fields,
		topicTemplate:     topicTemplate,
		resultTopicTmpl:   "event/irrigationResult/{field}/{sensor}",
		sensorLivenessTTL: 60 * time.Second,
		offlineGrace:      5 * time.Second,
	}
}

func (h *GrpcHandler) SetResultTopicTemplate(t string) {
	if strings.TrimSpace(t) != "" {
		h.resultTopicTmpl = t
	}
}

// SetLiveness imposta TTL di liveness e finestra di grace (richiamato dal main via ENV).
func (h *GrpcHandler) SetLiveness(ttl, grace time.Duration) {
	if ttl > 0 {
		h.sensorLivenessTTL = ttl
	}
	if grace > 0 {
		h.offlineGrace = grace
	}
}

// ============== RPC: StartIrrigation ==============

func (h *GrpcHandler) StartIrrigation(_ context.Context, req *pb.StartRequest) (*pb.CommandResponse, error) {
	fid, sid := strings.TrimSpace(req.GetFieldId()), strings.TrimSpace(req.GetSensorId())

	// lookup sensore (helper locale)
	sensor, ok := h.lookupSensor(fid, sid)
	if !ok {
		return &pb.CommandResponse{Success: false, Message: fmt.Sprintf("unknown field/sensor %s/%s", fid, sid)}, nil
	}

	// calcolo duration
	var durMin int32
	switch {
	case req.GetDurationMin() > 0:
		durMin = req.GetDurationMin()
	case req.GetAmountMm() > 0:
		mmPerMin := mmPerMinute(sensor)
		if mmPerMin <= 0 {
			mmPerMin = 10.0 / 60.0 // fallback 10 mm/h
		}
		durMin = int32(math.Ceil(req.GetAmountMm() / mmPerMin))
	default:
		durMin = 15
	}
	if durMin <= 0 {
		durMin = 1
	}

	// 1) publish StateChange ON
	stateEvt := model.StateChangeEvent{
		FieldID:   fid,
		SensorID:  sid,
		NewState:  model.StateOn,
		Duration:  time.Duration(durMin) * time.Minute,
		Timestamp: time.Now(),
	}
	b, _ := json.Marshal(stateEvt)
	stateTopic := formatTopic(h.topicTemplate, fid, sid)
	if err := h.makePublisher(stateTopic).PublishMessageQos(1, false, string(b)); err != nil {
		return &pb.CommandResponse{Success: false, Message: "publish state ON failed"}, err
	}

	// 2) Accepted + ticket, e publish Result al termine (con liveness reale e mm accumulati)
	ticket := uuid.New().String()
	started := time.Now()
	totalDur := time.Duration(durMin) * time.Minute
	requestedMM := req.GetAmountMm()

	mmPerMin := mmPerMinute(sensor)
	if mmPerMin <= 0 {
		mmPerMin = 10.0 / 60.0 // fallback
	}

	go func(fieldID, sensorID, decisionID, ticketID string, tot time.Duration) {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()

		deadline := time.Now().Add(tot)
		mmApplied := 0.0

		for time.Now().Before(deadline) {
			<-tick.C

			// liveness: se non riceviamo heartbeat entro TTL+grace -> FAIL con parziale
			if !h.isLive(fieldID, sensorID) && !h.waitGraceAlive(fieldID, sensorID, h.offlineGrace) {
				h.publishResult(messages.IrrigationResultEvent{
					FieldID:    fieldID,
					SensorID:   sensorID,
					TicketID:   ticketID,
					DecisionID: decisionID,
					Status:     "FAIL",
					MmApplied:  mmApplied,
					Reason:     "offline",
					StartedAt:  started,
					Timestamp:  time.Now(),
				})
				// OFF di sicurezza
				offEvt := model.StateChangeEvent{
					FieldID:   fieldID,
					SensorID:  sensorID,
					NewState:  model.StateOff,
					Duration:  0,
					Timestamp: time.Now(),
				}
				ob, _ := json.Marshal(offEvt)
				_ = h.makePublisher(stateTopic).PublishMessageQos(1, false, string(ob))
				return
			}

			// avanza irrigazione (mm/min -> mm/sec)
			mmApplied += mmPerMin / 60.0

			// se è stata richiesta una quantità specifica, opzionalmente clamp per tolleranza
			if requestedMM > 0 && mmApplied >= requestedMM {
				mmApplied = requestedMM
				break
			}
		}

		// fine naturale: OK
		h.publishResult(messages.IrrigationResultEvent{
			FieldID:    fieldID,
			SensorID:   sensorID,
			TicketID:   ticketID,
			DecisionID: decisionID,
			Status:     "OK",
			MmApplied:  mmApplied,
			Reason:     "done",
			StartedAt:  started,
			Timestamp:  time.Now(),
		})

		// OFF a fine ciclo
		offEvt := model.StateChangeEvent{
			FieldID:   fieldID,
			SensorID:  sensorID,
			NewState:  model.StateOff,
			Duration:  0,
			Timestamp: time.Now(),
		}
		ob, _ := json.Marshal(offEvt)
		_ = h.makePublisher(stateTopic).PublishMessageQos(1, false, string(ob))
	}(fid, sid, req.GetDecisionId(), ticket, totalDur)

	return &pb.CommandResponse{
		Success:  true,
		Message:  fmt.Sprintf("irrigation started for %s/%s (duration=%d min)", fid, sid, durMin),
		TicketId: ticket,
	}, nil
}

// ============== RPC: StopIrrigation ==============

func (h *GrpcHandler) StopIrrigation(_ context.Context, req *pb.StopRequest) (*pb.CommandResponse, error) {
	fid, sid := req.GetFieldId(), req.GetSensorId()
	evt := model.StateChangeEvent{
		FieldID:   fid,
		SensorID:  sid,
		NewState:  model.StateOff,
		Duration:  0,
		Timestamp: time.Now(),
	}
	b, _ := json.Marshal(evt)
	topic := formatTopic(h.topicTemplate, fid, sid)
	if err := h.makePublisher(topic).PublishMessageQos(1, false, string(b)); err != nil {
		return &pb.CommandResponse{Success: false, Message: "publish state OFF failed"}, err
	}
	return &pb.CommandResponse{Success: true, Message: fmt.Sprintf("irrigation stopped for %s/%s", fid, sid)}, nil
}

// ============== Helpers ==============

// OnSensorData aggiorna liveness da heartbeat implicito (sensor/data/+/+).
func (h *GrpcHandler) OnSensorData(_ string, m mqtt.Message) error {
	// topic atteso: sensor/data/{field}/{sensor} (configurabile dal main)
	parts := strings.Split(m.Topic(), "/")
	if len(parts) >= 4 {
		h.lastSeen.Store(parts[2]+"|"+parts[3], time.Now())
	}
	return nil
}

func (h *GrpcHandler) isLive(fieldID, sensorID string) bool {
	if v, ok := h.lastSeen.Load(fieldID + "|" + sensorID); ok {
		return time.Since(v.(time.Time)) < h.sensorLivenessTTL
	}
	return false
}

func (h *GrpcHandler) waitGraceAlive(fieldID, sensorID string, grace time.Duration) bool {
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if h.isLive(fieldID, sensorID) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func (h *GrpcHandler) publishResult(evt messages.IrrigationResultEvent) {
	topic := strings.NewReplacer("{field}", evt.FieldID, "{sensor}", evt.SensorID).
		Replace(firstNonEmpty(h.resultTopicTmpl, "event/irrigationResult/{field}/{sensor}"))
	payload, _ := json.Marshal(evt)
	_ = h.makePublisher(topic).PublishMessageQos(1, false, string(payload))
}

func formatTopic(tmpl, fieldID, sensorID string) string {
	if strings.TrimSpace(tmpl) == "" {
		tmpl = "event/StateChange/{field}/{sensor}"
	}
	return strings.NewReplacer("{field}", fieldID, "{sensor}", sensorID).Replace(tmpl)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// lookupSensor: evita dipendere da metodi non presenti su model.Field
func (h *GrpcHandler) lookupSensor(fieldID, sensorID string) (model.Sensor, bool) {
	f, ok := h.fields[fieldID]
	if !ok {
		return model.Sensor{}, false
	}
	for _, s := range f.Sensors {
		if s.ID == sensorID {
			return s, true
		}
	}
	return model.Sensor{}, false
}

// 1 L/m2 = 1 mm
func mmPerMinute(s model.Sensor) float64 {
	if s.AreaM2 <= 0 || s.FlowLpm <= 0 {
		return 0
	}
	return s.FlowLpm / s.AreaM2
}
