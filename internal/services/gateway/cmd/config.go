package main

import (
	"os"
	"strconv"
)

type Config struct {
	Port      string
	TimeoutMs int

	DeviceURL      string
	PersistenceURL string
	EventURL       string
	AnalyticsURL   string

	// Circuit Breaker: Event service
	CBEventFails      int // # fallimenti consecutivi per aprire
	CBEventOpenMs     int // finestra OPEN (ms)
	CBEventIntervalMs int // finestra di calcolo contatori (ms)

	// Circuit Breaker: REST backends (device/persistence/analytics)
	CBRestFails      int
	CBRestOpenMs     int
	CBRestIntervalMs int
}

func loadConfig() Config {
	return Config{
		Port:      getenv("PORT", "5009"),
		TimeoutMs: getenvInt("TIMEOUT_MS", 3000),

		DeviceURL:      getenv("DEVICE_URL", "http://device-service.fog:8080"),
		PersistenceURL: getenv("PERSISTENCE_URL", "http://persistence.cloud:8080"),
		EventURL:       getenv("EVENT_URL", "http://event-service.fog:8080"),
		AnalyticsURL:   getenv("ANALYTICS_URL", "http://analytics.cloud:8080"),

		// Circuit Breaker defaults
		CBEventFails:      getenvInt("CB_EVENT_FAILS", 3),
		CBEventOpenMs:     getenvInt("CB_EVENT_OPEN_MS", 15000),
		CBEventIntervalMs: getenvInt("CB_EVENT_INTERVAL_MS", 60000),

		CBRestFails:      getenvInt("CB_REST_FAILS", 3),
		CBRestOpenMs:     getenvInt("CB_REST_OPEN_MS", 15000),
		CBRestIntervalMs: getenvInt("CB_REST_INTERVAL_MS", 60000),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}
