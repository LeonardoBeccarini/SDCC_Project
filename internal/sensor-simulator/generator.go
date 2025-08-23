package sensor_simulator

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/LeonardoBeccarini/sdcc_project/internal/model"
)

// DataGenerator just knows how to fetch & decay soil-moisture.
type DataGenerator struct {
	lastMoisture  float64   // 0.0–1.0
	lastTimestamp time.Time // when lastMoisture was set
	decayRate     float64   // per second
}

// NewDataGenerator returns a generator with your chosen decayRate.
func NewDataGenerator(decayRate float64) *DataGenerator {
	return &DataGenerator{decayRate: decayRate}
}

// Next pulls new raw data, applies decay since last timestamp,
// and returns a complete model.SensorData.
func (g *DataGenerator) Next(sensor *model.Sensor) (model.SensorData, error) {
	// 1) Hit the SoilGrids API
	sd, err := fetchSensorData(*sensor)
	if err != nil {
		return model.SensorData{}, err
	}

	now := time.Now().UTC()
	if !g.lastTimestamp.IsZero() {
		dt := now.Sub(g.lastTimestamp).Seconds()
		decayed := g.lastMoisture * math.Exp(-g.decayRate*dt)
		sd.Moisture = int(math.Round(decayed * 100))
		g.lastMoisture = decayed
	} else {
		// first run: seed from API value
		g.lastMoisture = float64(sd.Moisture) / 100.0
	}
	g.lastTimestamp = now

	return sd, nil
}

// SoilGridResponse matches the actual JSON shape returned by SoilGrids.
type SoilGridResponse struct {
	Properties struct {
		Layers []struct {
			Name   string `json:"name"`
			Depths []struct {
				Range struct {
					TopDepth    int `json:"top_depth"`    // cm
					BottomDepth int `json:"bottom_depth"` // cm
				} `json:"range"`
				Values struct {
					Mean float64 `json:"mean"` // mapped units (×10)
				} `json:"values"`
			} `json:"depths"`
		} `json:"layers"`
	} `json:"properties"`
}

// fetchSensorData pulls the SoilGrids wv0010 property, picks the
// thickest layer up to maxDepth (fallback: first layer), converts to an
// integer percent, and wraps it in your model.SensorData.
func fetchSensorData(sensor model.Sensor) (model.SensorData, error) {
	// 1. Hit the API asking only for wv0010
	url := fmt.Sprintf(
		"https://rest.isric.org/soilgrids/v2.0/properties/query?lat=%f&lon=%f&property=wv0010",
		sensor.Latitude, sensor.Longitude,
	)
	resp, err := http.Get(url)
	if err != nil {
		return model.SensorData{}, fmt.Errorf("HTTP error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return model.SensorData{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	// 2. Decode into our struct
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return model.SensorData{}, fmt.Errorf("read body: %w", err)
	}

	var sg SoilGridResponse
	if err := json.Unmarshal(body, &sg); err != nil {
		return model.SensorData{}, fmt.Errorf("unmarshal: %w\nJSON: %s", err, string(body))
	}

	// 3. Find the thickest layer ≤ maxDepth (fallback: first)
	layers := sg.Properties.Layers
	if len(layers) == 0 {
		return model.SensorData{}, fmt.Errorf("no layers in response")
	}
	depths := layers[0].Depths
	if len(depths) == 0 {
		return model.SensorData{}, fmt.Errorf("no depths in first layer")
	}

	var bestMean float64
	var bestThick int
	for _, d := range depths {
		if d.Range.BottomDepth <= sensor.MaxDepth {
			thick := d.Range.BottomDepth - d.Range.TopDepth
			if thick > bestThick {
				bestThick = thick
				bestMean = d.Values.Mean
			}
		}
	}
	// Fallback: if nothing ≤ MaxDepth, take the first depth entry
	if bestThick == 0 {
		bestMean = depths[0].Values.Mean
	}

	// 4. Convert mapped units (e.g. 314 → 31.4%) to an int percent
	moisturePct := int(math.Round(bestMean / 10))

	// 5. Build and return your SensorData
	return model.SensorData{
		SensorID:   sensor.ID,
		FieldID:    sensor.FieldID,
		Moisture:   moisturePct,
		Aggregated: false,
		Timestamp:  time.Now().UTC(),
	}, nil
}

/*
ApplyIrrigation bumps the internal moisture state proportionally to the
irrigation duration. Simple linear model: +0.3 percentage point per minute.
e.g., 10 minutes -> +3.0%. Values are clamped to [0, 1].
*/
func (g *DataGenerator) ApplyIrrigation(d time.Duration) {
	if g == nil || d <= 0 {
		return
	}
	// 0.003 in [0..1] units per minute = 0.3%/min
	inc := 0.003 * d.Minutes()
	g.lastMoisture += inc
	if g.lastMoisture > 1.0 {
		g.lastMoisture = 1.0
	}
	if g.lastMoisture < 0.0 {
		g.lastMoisture = 0.0
	}
}
