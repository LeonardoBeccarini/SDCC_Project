package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/LeonardoBeccarini/sdcc_project/internal/services/gateway/app"
)

func getenv(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func getenvInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	var n int
	_, err := fmt.Sscanf(v, "%d", &n)
	if err != nil {
		return def
	}
	return n
}

func main() {
	// Config da env (default coerenti)
	persistenceURL := strings.TrimRight(getenv("PERSISTENCE_URL", "http://localhost:8080"), "/")
	eventsURL := strings.TrimRight(getenv("EVENT_URL", ""), "/")

	httpTimeout := time.Duration(getenvInt("HTTP_TIMEOUT_MS", 3000)) * time.Millisecond
	breakerFailures := getenvInt("BREAKER_FAILURES", 3)
	breakerOpenMS := getenvInt("BREAKER_OPEN_MS", 15000)

	// Path chiamati lato upstream (override via env se serve)
	persistencePath := getenv("PERSISTENCE_PATH", "/data/latest")
	// ✅ corretto: allineato al servizio Event
	eventsPath := getenv("EVENTS_IRRIGATIONS_PATH", "/events/irrigation/latest?limit=20")

	logger := log.New(os.Stdout, "", log.LstdFlags)

	cfg := app.Config{
		PersistenceBaseURL: persistenceURL,
		EventsBaseURL:      eventsURL,
		PersistencePath:    persistencePath,
		EventsPath:         eventsPath,
		HTTPTimeout:        httpTimeout,
		BreakerFailures:    breakerFailures,
		BreakerOpenFor:     time.Duration(breakerOpenMS) * time.Millisecond,
		Logger:             logger,
	}

	gw := app.NewGateway(cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/dashboard/data", gw.HandleDashboard)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })

	addr := ":" + getenv("PORT", "5009")
	logger.Printf("gateway listening on %s (persistence=%s, events=%s)", addr, persistenceURL, eventsURL)
	log.Fatal(http.ListenAndServe(addr, mux))
}
