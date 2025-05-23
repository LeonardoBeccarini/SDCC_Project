package device

import (
	"context"
	"encoding/json"
	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"log"
	"time"
)

type DeviceService struct {
	consumer  rabbitmq.IConsumer[model.SensorData] //consumes data from sensors for local check
	publisher rabbitmq.IPublisher                  // publishes state changes
	fields    map[string]model.Field               // fields this service controls
}

func NewDeviceService(consumer rabbitmq.IConsumer[model.SensorData], publisher rabbitmq.IPublisher, fields map[string]model.Field) *DeviceService {
	return &DeviceService{
		consumer:  consumer,
		publisher: publisher,
		fields:    fields,
	}
}

func (d *DeviceService) Start(ctx context.Context) {
	// 1) inject the handler
	d.consumer.SetHandler(d.messageHandler)

	// 2) ensure publisher is closed when we exit
	defer d.publisher.Close()

	// 3) start the consume loop — this should block until ctx is cancelled
	d.consumer.ConsumeMessage(ctx)

}

func (d *DeviceService) messageHandler(_ string, message mqtt.Message) error {
	ctx := context.Background()

	// 1) Decode the incoming reading
	var sd model.SensorData
	if err := json.Unmarshal(message.Payload(), &sd); err != nil {
		log.Printf("Error unmarshalling sensor data: %v", err)
		return err
	}

	log.Printf("Received sensor data: FieldID=%s, SensorID=%s, Moisture=%d", sd.FieldID, sd.SensorID, sd.Moisture)

	// 2) Load the Field (with its sensors)
	field, ok := d.fields[sd.FieldID]
	if !ok {
		log.Printf("Unknown field %s; skipping", sd.FieldID)
		return nil
	}

	// 3) Find the specific Sensor in that Field
	sensorPtr := field.GetSensor(sd.SensorID)
	if sensorPtr == nil {
		log.Printf("Sensor %s not found in field %s; skipping", sd.SensorID, sd.FieldID)
		return nil
	}

	// 4) Fetch the policy (user‐defined threshold & water quantity)
	policy, err := d.getUserPolicy(ctx, sd.FieldID)
	if err != nil {
		log.Printf("Error retrieving policy for field %s: %v", sd.FieldID, err)
		return err
	}

	// 5) If this sensor’s reading is below threshold, activate it
	if sd.Moisture < policy.MoistureThreshold {
		// Compute exactly how long to open the valve for this sensor
		duration := sensorPtr.ComputeDuration(policy.WaterQuantity)

		log.Printf("Moisture %d below threshold %d, activating irrigation for Sensor %s, Duration: %s", sd.Moisture, policy.MoistureThreshold, sensorPtr.ID, duration)

		// Flip its in-memory state (optional, if you need to track it)
		field.ToggleIrrigation(sensorPtr.ID, true)

		// Build and send a single StateChangeEvent
		evt := model.StateChangeEvent{
			FieldID:   sd.FieldID,
			SensorID:  sensorPtr.ID,
			NewState:  model.StateOn,
			Duration:  duration,
			Timestamp: time.Now(),
		}
		b, err := json.Marshal(evt)
		if err != nil {
			log.Printf("Error marshalling event for %s/%s: %v", sd.FieldID, sensorPtr.ID, err)
			return nil
		}
		if err := d.publisher.PublishMessage(string(b)); err != nil {
			log.Printf("Error publishing event for %s/%s: %v", sd.FieldID, sensorPtr.ID, err)
		} else {
			log.Printf("Activated %s/%s for %s", sd.FieldID, sensorPtr.ID, duration)
		}
	}

	return nil
}

// getUserPolicy simulates fetching the policy associated with a given FieldID.
// In a real implementation this might query a database or HTTP API.
func (d *DeviceService) getUserPolicy(ctx context.Context, fieldID string) (model.Policy, error) {
	// --- mimic logic: choose threshold based on field ---
	switch fieldID {
	case "field1":
		return model.Policy{MoistureThreshold: 25, WaterQuantity: 40}, nil
	case "field2":
		return model.Policy{MoistureThreshold: 40, WaterQuantity: 15}, nil
	default:
		return model.Policy{MoistureThreshold: 30}, nil
	}
}

// TODO bisogna capire bene come gestire l'accensione e lo spegnimento dei sensori,
// TODO cioè serve un micorservizio che gestisca lo scheduling
//TODO
