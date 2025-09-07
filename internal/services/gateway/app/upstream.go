package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Upstream incapsula chiamate HTTP con Circuit Breaker
type Upstream struct {
	base    string
	path    string
	client  *http.Client
	breaker *CircuitBreaker
	name    string
}

// NewUpstream costruisce un client verso un servizio a monte
func NewUpstream(name, base, path string, timeout time.Duration, breaker *CircuitBreaker) *Upstream {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	path = "/" + strings.TrimLeft(strings.TrimSpace(path), "/")
	return &Upstream{
		base:    base,
		path:    path,
		client:  &http.Client{Timeout: timeout},
		breaker: breaker,
		name:    name,
	}
}

// GetJSON esegue la GET e decodifica JSON in out
func (u *Upstream) GetJSON(ctx context.Context, out any) error {
	if u == nil || u.base == "" {
		// upstream opzionale non configurato: non è un errore, lasciamo out invariato
		return nil
	}
	if !u.breaker.Allow() {
		return fmt.Errorf("%s breaker open", u.name)
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.base+u.path, nil)
	resp, err := u.client.Do(req)
	if err != nil {
		u.breaker.Fail()
		return fmt.Errorf("%s request error: %w", u.name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		u.breaker.Fail()
		return fmt.Errorf("%s upstream status %d", u.name, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		u.breaker.Fail()
		return fmt.Errorf("%s decode error: %w", u.name, err)
	}

	u.breaker.Success()
	return nil
}
