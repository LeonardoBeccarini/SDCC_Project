package main

import (
	"os"
	"strconv"
)

type Config struct {
	Port         string
	InfluxURL    string
	InfluxToken  string
	InfluxOrg    string
	InfluxBucket string
	MetricName   string
	TimeoutMs    int

	// Fallback microservizi (opzionali)
	DeviceURL      string // es. http://device-service.fog:8080
	PersistenceURL string // es. http://persistence.cloud:8080
	EventURL       string // es. http://event-service.fog:8080
	AnalyticsURL   string // es. http://analytics.cloud:8080
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func getenvInt(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return d
}

func loadConfig() Config {
	return Config{
		Port:         getenv("PORT", "5009"),
		InfluxURL:    getenv("INFLUX_URL", "http://influxdb:8086"),
		InfluxToken:  getenv("INFLUX_TOKEN", ""),
		InfluxOrg:    getenv("INFLUX_ORG", "sdcc"),
		InfluxBucket: getenv("INFLUX_BUCKET", "agri"),
		MetricName:   getenv("METRIC_NAME", "soil_moisture"),
		TimeoutMs:    getenvInt("TIMEOUT_MS", 3000),

		DeviceURL:      getenv("DEVICE_URL", "http://device-service.fog:8080"),
		PersistenceURL: getenv("PERSISTENCE_URL", "http://persistence.cloud:8080"),
		EventURL:       getenv("EVENT_URL", "http://event-service.fog:8080"),
		AnalyticsURL:   getenv("ANALYTICS_URL", "http://analytics.cloud:8080"),
	}
}
