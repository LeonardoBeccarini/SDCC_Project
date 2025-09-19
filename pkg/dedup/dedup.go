package dedup

import (
	"sync"
	"time"
)

type Deduper struct {
	mu   sync.Mutex
	ttl  time.Duration
	max  int
	seen map[string]time.Time
}

func New(ttl time.Duration, max int) *Deduper {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if max <= 0 {
		max = 10000
	}
	return &Deduper{ttl: ttl, max: max, seen: make(map[string]time.Time, max)}
}

func (d *Deduper) ShouldProcess(id string) bool {
	if id == "" {
		return true
	}
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()
	if exp, ok := d.seen[id]; ok && now.Before(exp) {
		return false
	}
	d.seen[id] = now.Add(d.ttl)
	if len(d.seen) > d.max {
		for k, v := range d.seen {
			if now.After(v) {
				delete(d.seen, k)
			}
			if len(d.seen) <= d.max {
				break
			}
		}
	}
	return true
}
