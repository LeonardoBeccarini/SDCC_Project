package irrigation_controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

type owmDaily struct {
	Dt   int64 `json:"dt"`
	Temp struct {
		Min float64 `json:"min"`
		Max float64 `json:"max"`
	} `json:"temp"`
	Rain float64 `json:"rain"`
}

type owmResp struct {
	Daily []owmDaily `json:"daily"`
}

type OWMClient struct{ apiKey string }

func NewOWMClient(key string) *OWMClient { return &OWMClient{apiKey: key} }

// Implementa l'interfaccia WeatherClient: accetta anche 'day'
func (c *OWMClient) GetDailyET0AndRain(ctx context.Context, lat, lon float64, day time.Time) (float64, float64, error) {
	if c.apiKey == "" {
		return 0, 0, fmt.Errorf("missing api key")
	}
	url := fmt.Sprintf("https://api.openweathermap.org/data/3.0/onecall?lat=%f&lon=%f&exclude=current,minutely,hourly,alerts&units=metric&appid=%s", lat, lon, c.apiKey)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return 0, 0, fmt.Errorf("owm status %d: %s", resp.StatusCode, string(b))
	}
	var out owmResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, 0, err
	}
	if len(out.Daily) == 0 {
		return 0, 0, fmt.Errorf("no daily data")
	}

	// Scegli il giorno più vicino a 'day' (UTC) tra i daily restituiti
	target := time.Date(day.UTC().Year(), day.UTC().Month(), day.UTC().Day(), 0, 0, 0, 0, time.UTC)
	chosen := out.Daily[0]
	minDelta := time.Duration(1<<63 - 1)
	for _, d := range out.Daily {
		t := time.Unix(d.Dt, 0).UTC()
		date := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		delta := target.Sub(date)
		if delta < 0 {
			delta = -delta
		}
		if delta < minDelta {
			minDelta = delta
			chosen = d
		}
	}

	// Ra costante semplificata per ottenere mm/giorno (approssimazione)
	ra := 0.408
	et0 := etoHargreaves(chosen.Temp.Min, chosen.Temp.Max, ra)
	return et0, chosen.Rain, nil
}

// Hargreaves semplificato
func etoHargreaves(tmin, tmax, ra float64) float64 {
	tmean := (tmin + tmax) / 2.0
	return 0.0023 * (tmean + 17.8) * math.Sqrt(math.Max(tmax-tmin, 0)) * ra
}
