package event

import (
	"log"
	"sync"
	"time"

	"github.com/influxdata/influxdb-client-go/v2/api"
)

// Writer incapsula WriteAPI e traccia l'ultimo errore di scrittura per /healthz e /readyz.
type Writer struct {
	api     api.WriteAPI
	mu      sync.RWMutex
	lastErr time.Time
	counts  map[string]int64
}

// NewWriter inizializza il writer e attiva il listener degli errori asincroni di Influx.
func NewWriter(w api.WriteAPI) *Writer {
	ww := &Writer{
		api:     w,
		lastErr: time.Now().Add(-24 * time.Hour), // di default "lontano nel tempo"
		counts:  make(map[string]int64),
	}
	go func() {
		for err := range w.Errors() {
			if err != nil {
				ww.mu.Lock()
				ww.lastErr = time.Now()
				ww.mu.Unlock()
				log.Printf("influx write error: %v", err)
			}
		}
	}()
	return ww
}

// LastErrorAge ritorna da quanto tempo non si verificano errori di scrittura.
func (w *Writer) LastErrorAge() time.Duration {
	if w == nil {
		// se per qualche motivo non è stato inizializzato, ritorna un'età grande
		return 99999 * time.Hour
	}
	w.mu.RLock()
	t := w.lastErr
	w.mu.RUnlock()
	return time.Since(t)
}

// MarkIngest incrementa un contatore interno per metrica grezza (non essenziale, ma utile al debug).
func (w *Writer) MarkIngest(eventType string) {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.counts[eventType]++
	w.mu.Unlock()
}

// Count permette di leggere il contatore per tipo evento. (opzionale)
func (w *Writer) Count(eventType string) int64 {
	if w == nil {
		return 0
	}
	w.mu.RLock()
	c := w.counts[eventType]
	w.mu.RUnlock()
	return c
}
