package tastytrade

import (
	"sync"
	"time"
)

// openInterestCacheTTL: el open interest se recalcula una vez por dia en
// OCC -- 6h de cache alcanza para no repetir el doble REST por simbolo en
// cada poll del frontend.
const (
	openInterestCacheTTL = 6 * time.Hour
	openInterestErrorTTL = 10 * time.Minute
)

type openInterestEntry struct {
	expires time.Time
	value   float64
	found   bool
	err     error
}

type openInterestCache struct {
	mu      sync.Mutex
	entries map[string]openInterestEntry
}

func (c *openInterestCache) get(symbol string) (float64, bool, error, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[symbol]
	if !ok {
		return 0, false, nil, false
	}
	if time.Now().After(entry.expires) {
		delete(c.entries, symbol)
		return 0, false, nil, false
	}
	return entry.value, entry.found, entry.err, true
}

func (c *openInterestCache) put(symbol string, value float64, found bool) {
	c.putWithTTL(symbol, value, found, nil, openInterestCacheTTL)
}

func (c *openInterestCache) putError(symbol string, err error) {
	c.putWithTTL(symbol, 0, false, err, openInterestErrorTTL)
}

func (c *openInterestCache) putWithTTL(symbol string, value float64, found bool, err error, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[symbol] = openInterestEntry{expires: time.Now().Add(ttl), value: value, found: found, err: err}
}
