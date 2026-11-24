package tastytrade

import (
	"sync"
	"time"
)

// openInterestCacheTTL: el open interest se recalcula una vez por dia en
// OCC -- 6h de cache alcanza para no repetir el doble REST por simbolo en
// cada poll del frontend.
const openInterestCacheTTL = 6 * time.Hour

type openInterestEntry struct {
	expires time.Time
	value   float64
	found   bool
}

type openInterestCache struct {
	mu      sync.Mutex
	entries map[string]openInterestEntry
}

func (c *openInterestCache) get(symbol string) (float64, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[symbol]
	if !ok {
		return 0, false, false
	}
	if time.Now().After(entry.expires) {
		delete(c.entries, symbol)
		return 0, false, false
	}
	return entry.value, entry.found, true
}

func (c *openInterestCache) put(symbol string, value float64, found bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[symbol] = openInterestEntry{expires: time.Now().Add(openInterestCacheTTL), value: value, found: found}
}
