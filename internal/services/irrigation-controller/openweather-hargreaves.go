package irrigation_controller

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"
)

type OWMClient struct {
	apiKey string
	http   *http.Client
}

func NewOWMClient(apiKey string) *OWMClient {
	return &OWMClient{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 5 * time.Second},
	}
}

type onecallDaily struct {
	Dt   int64 `json:"dt"`
	Temp struct {
		Min float64 `json:"min"`
		Max float64 `json:"max"`
	} `json:"temp"`
	Rain float64 `json:"rain"`
}

type onecallResp struct {
	Daily []onecallDaily `json:"daily"`
}

func (c *OWMClient) GetDailyET0AndRain(ctx context.Context, lat, lon float64, day time.Time) (float64, float64, error) {
	if c == nil || c.apiKey == "" {
		return 0, 0, fmt.Errorf("OWM apiKey missing")
	}
	url := fmt.Sprintf("https://api.openweathermap.org/data/3.0/onecall?lat=%f&lon=%f&exclude=minutely,hourly,alerts,current&units=metric&appid=%s", lat, lon, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, 0, fmt.Errorf("owm status %d", resp.StatusCode)
	}

	var payload onecallResp
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, 0, err
	}

	target := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	var Tmin, Tmax, rain float64
	for _, d := range payload.Daily {
		dt := time.Unix(d.Dt, 0).UTC()
		if dt.Year() == target.Year() && dt.YearDay() == target.YearDay() {
			Tmin, Tmax, rain = d.Temp.Min, d.Temp.Max, d.Rain
			break
		}
	}
	if Tmax == 0 && Tmin == 0 && len(payload.Daily) > 0 {
		Tmin, Tmax, rain = payload.Daily[0].Temp.Min, payload.Daily[0].Temp.Max, payload.Daily[0].Rain
	}

	Tmean := (Tmin + Tmax) / 2.0
	Trange := Tmax - Tmin
	if Trange < 0 {
		Trange = 0
	}
	Ra := extraterrestrialRadiation(lat, target) // MJ/m2/day
	ETo := 0.0023 * (Tmean + 17.8) * math.Sqrt(Trange) * Ra * 0.408

	return ETo, rain, nil
}

func extraterrestrialRadiation(lat float64, day time.Time) float64 {
	phi := lat * math.Pi / 180.0
	doy := float64(day.YearDay())
	dr := 1 + 0.033*math.Cos(2*math.Pi*doy/365)
	delta := 0.409 * math.Sin(2*math.Pi*doy/365-1.39)
	ws := math.Acos(-math.Tan(phi) * math.Tan(delta))
	Gsc := 0.0820
	Ra := (24 * 60 / math.Pi) * Gsc * dr * (ws*math.Sin(phi)*math.Sin(delta) + math.Cos(phi)*math.Cos(delta)*math.Sin(ws))
	return Ra
}
