package device

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	pb "github.com/LeonardoBeccarini/sdcc_project/deploy/gen/go/irrigation"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
)

type GrpcHandler struct {
	pb.UnimplementedDeviceServiceServer
	publisher rabbitmq.IPublisher
	fields    map[string]model.Field
}

func NewGrpcHandler(publisher rabbitmq.IPublisher, fields map[string]model.Field) *GrpcHandler {
	return &GrpcHandler{publisher: publisher, fields: fields}
}

func (h *GrpcHandler) StartIrrigation(ctx context.Context, req *pb.StartRequest) (*pb.CommandResponse, error) {
	log.Printf("gRPC: Start irrigation for %s (%d minutes)", req.FieldId, req.DurationMin)

	event := model.StateChangeEvent{
		FieldID:   req.FieldId,
		SensorID:  "all",
		NewState:  model.StateOn,
		Duration:  time.Duration(req.DurationMin) * time.Minute,
		Timestamp: time.Now(),
	}

	b, err := json.Marshal(event)
	if err != nil {
		return &pb.CommandResponse{Success: false, Message: "Marshal failed"}, err
	}

	if err := h.publisher.PublishMessage(string(b)); err != nil {
		return &pb.CommandResponse{Success: false, Message: "Publish failed"}, err
	}

	return &pb.CommandResponse{Success: true, Message: fmt.Sprintf("Irrigation started for field %s", req.FieldId)}, nil
}

func (h *GrpcHandler) StopIrrigation(ctx context.Context, req *pb.StopRequest) (*pb.CommandResponse, error) {
	log.Printf("gRPC: Stop irrigation for %s", req.FieldId)

	event := model.StateChangeEvent{
		FieldID:   req.FieldId,
		SensorID:  "all",
		NewState:  model.StateOff,
		Duration:  0,
		Timestamp: time.Now(),
	}

	b, err := json.Marshal(event)
	if err != nil {
		return &pb.CommandResponse{Success: false, Message: "Marshal failed"}, err
	}

	if err := h.publisher.PublishMessage(string(b)); err != nil {
		return &pb.CommandResponse{Success: false, Message: "Publish failed"}, err
	}

	return &pb.CommandResponse{Success: true, Message: fmt.Sprintf("Irrigation stopped for field %s", req.FieldId)}, nil
}
