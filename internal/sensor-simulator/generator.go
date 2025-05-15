package sensor_simulator

import (
	"context"
	"encoding/json"
	"github.com/LeonardoBeccarini/sdcc_project/pkg/rabbitmq"
	"log"
	"math/rand"
	"time"

	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
)

type SensorService struct {
	publisher rabbitmq.IPublisher
}

func NewSensorService(publisher rabbitmq.IPublisher) *SensorService {
	return &SensorService{publisher: publisher}
}
func (s *SensorService) StartPublishing(ctx context.Context, interval time.Duration) {
	for {
		select {
		case <-ctx.Done():
			s.publisher.Close()
			return
		case <-time.After(interval):
			msgBytes, err := GenerateSoilMoisture("sensor1")

			// Create a message based on the generated moisture data
			message := string(msgBytes)

			// Publish the generated moisture data as a message
			err = s.publisher.PublishMessage(message)
			if err != nil {
				log.Printf("Error publishing message: %v", err)
			}
		}
	}
}

func GenerateSoilMoisture(sensorID string) ([]byte, error) {
	// Seed the random number generator to get different results each time
	rand.Seed(time.Now().UnixNano())

	// Initial moisture level (in percentage)
	moistureLevel := rand.Intn(101) // Start with a random moisture level between 0 and 100

	// Start a ticker to run every 15 minutes (15 * 60 seconds)
	ticker := time.NewTicker(10 * time.Second)

	// Run the loop indefinitely to output moisture values every 15 minutes
	for {
		// Wait for the next tick
		<-ticker.C

		// Generate a realistic moisture change within a range of -10 to +10
		// Ensure moisture level stays between 0 and 100
		change := rand.Intn(21) - 10 // This will give values between -10 and +10

		// Apply the change and ensure the value stays within bounds (0 - 100)
		moistureLevel += change
		if moistureLevel > 100 {
			moistureLevel = 100
		} else if moistureLevel < 0 {
			moistureLevel = 0
		}
		log.Printf("Moisture level '%d' generated", moistureLevel)

		data := model.SensorData{
			SensorID:  sensorID,
			Moisture:  moistureLevel,
			Timestamp: time.Now(),
		}
		return json.Marshal(data)
	}

}
