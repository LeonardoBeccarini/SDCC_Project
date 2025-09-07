package app

import (
	"log"
	"time"
)

type Config struct {
	PersistenceBaseURL string
	EventsBaseURL      string
	PersistencePath    string
	EventsPath         string
	HTTPTimeout        time.Duration

	BreakerFailures int
	BreakerOpenFor  time.Duration

	Logger *log.Logger
}

type Gateway struct {
	cfg         Config
	persistence *Upstream
	events      *Upstream
}

func NewGateway(cfg Config) *Gateway {
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	// Un breaker per ciascun upstream
	pb := NewCircuitBreaker(cfg.BreakerFailures, cfg.BreakerOpenFor)
	eb := NewCircuitBreaker(cfg.BreakerFailures, cfg.BreakerOpenFor)

	p := NewUpstream("persistence", cfg.PersistenceBaseURL, cfg.PersistencePath, cfg.HTTPTimeout, pb)
	e := NewUpstream("events", cfg.EventsBaseURL, cfg.EventsPath, cfg.HTTPTimeout, eb)

	return &Gateway{cfg: cfg, persistence: p, events: e}
}
