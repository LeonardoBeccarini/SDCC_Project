package device

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	pb "github.com/LeonardoBeccarini/sdcc_project/grpc/gen/go/irrigation"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
)

type PublisherFactory func(topic string) rabbitmq.IPublisher

type GrpcHandler struct {
	pb.UnimplementedDeviceServiceServer
	makePublisher PublisherFactory
	fields        map[string]model.Field
	// Template del topic, es: "event/StateChange/{field}/{sensor}"
	topicTemplate string
}

func NewGrpcHandler(factory PublisherFactory, topicTemplate string, fields map[string]model.Field) *GrpcHandler {
	return &GrpcHandler{
		makePublisher: factory,
		fields:        fields,
		topicTemplate: topicTemplate,
	}
}

func (h *GrpcHandler) StartIrrigation(ctx context.Context, req *pb.StartRequest) (*pb.CommandResponse, error) {
	fid, sid := req.GetFieldId(), req.GetSensorId()
	s, ok := h.lookupSensor(fid, sid)
	if !ok {
		msg := fmt.Sprintf("unknown field/sensor: %s/%s", fid, sid)
		return &pb.CommandResponse{Success: false, Message: msg}, nil
	}

	// determina durata (minuti)
	var durMin int32
	switch {
	case req.GetDurationMin() > 0:
		durMin = req.GetDurationMin()
	case req.GetAmountMm() > 0:
		mmPerMin := mmPerMinute(*s)
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

	evt := model.StateChangeEvent{
		FieldID:   fid,
		SensorID:  sid,
		NewState:  model.StateOn,
		Duration:  time.Duration(durMin) * time.Minute,
		Timestamp: time.Now(),
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		return &pb.CommandResponse{Success: false, Message: "marshal failed"}, err
	}

	topic := formatTopic(h.topicTemplate, fid, sid)
	if err := h.makePublisher(topic).PublishMessage(string(payload)); err != nil {
		return &pb.CommandResponse{Success: false, Message: "publish failed"}, err
	}

	return &pb.CommandResponse{
		Success: true,
		Message: fmt.Sprintf("irrigation started for %s/%s (duration=%d min)", fid, sid, durMin),
	}, nil
}

func (h *GrpcHandler) StopIrrigation(ctx context.Context, req *pb.StopRequest) (*pb.CommandResponse, error) {
	fid, sid := req.GetFieldId(), req.GetSensorId()
	evt := model.StateChangeEvent{
		FieldID:   fid,
		SensorID:  sid,
		NewState:  model.StateOff,
		Duration:  0,
		Timestamp: time.Now(),
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		return &pb.CommandResponse{Success: false, Message: "marshal failed"}, err
	}

	topic := formatTopic(h.topicTemplate, fid, sid)
	if err := h.makePublisher(topic).PublishMessage(string(payload)); err != nil {
		return &pb.CommandResponse{Success: false, Message: "publish failed"}, err
	}

	return &pb.CommandResponse{
		Success: true,
		Message: fmt.Sprintf("irrigation stopped for %s/%s", fid, sid),
	}, nil
}

// ---- helpers ----

func (h *GrpcHandler) lookupSensor(fieldID, sensorID string) (*model.Sensor, bool) {
	f, ok := h.fields[fieldID]
	if !ok {
		return nil, false
	}
	for i := range f.Sensors {
		if f.Sensors[i].ID == sensorID {
			return &f.Sensors[i], true
		}
	}
	return nil, false
}

func mmPerMinute(s model.Sensor) float64 {
	area := 1.0
	if s.AreaM2 > 0 {
		area = s.AreaM2
	}
	flow := 0.0
	if s.FlowLpm > 0 {
		flow = s.FlowLpm
	}
	if area <= 0 || flow <= 0 {
		return 0
	}
	// 1 L/m2 = 1 mm → mm/min = Lpm / m2
	return flow / area
}

func formatTopic(tmpl, fieldID, sensorID string) string {
	if strings.TrimSpace(tmpl) == "" {
		tmpl = "event/StateChange/{field}/{sensor}"
	}
	r := strings.NewReplacer("{field}", fieldID, "{sensor}", sensorID)
	return r.Replace(tmpl)
}
