package tastytrade

import (
	"sync"
	"time"
)

// profileCollector acumula "shares" por simbolo de un lote -- dxFeed manda
// el snapshot inicial de cada simbolo suscripto en rafagas separadas en el
// tiempo (mismo comportamiento que el historial de velas, ver
// historyCollector), no todo de una.
type profileCollector struct {
	mu         sync.Mutex
	shares     map[string]float64
	received   int
	lastUpdate time.Time
}

func newProfileCollector() *profileCollector {
	return &profileCollector{shares: make(map[string]float64)}
}

func (c *profileCollector) onProfile(ev rawProfileEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ev.Shares != nil {
		if _, exists := c.shares[ev.Symbol]; !exists {
			c.received++
		}
		c.shares[ev.Symbol] = *ev.Shares
	}
	c.lastUpdate = time.Now()
}

// settled: terminado cuando ya llegaron todos los simbolos esperados con
// "shares", o paso el periodo de silencio sin nada nuevo -- algunos
// simbolos genuinamente no traen ese campo (ej. ETFs, indices), asi que
// "todos" nunca se cumple en la practica para un lote mixto.
func (c *profileCollector) settled(expected int, quietPeriod time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.received >= expected {
		return true
	}
	if c.lastUpdate.IsZero() {
		return false
	}
	return time.Since(c.lastUpdate) >= quietPeriod
}

func (c *profileCollector) result() map[string]int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[string]int64, len(c.shares))
	for symbol, shares := range c.shares {
		result[symbol] = int64(shares)
	}
	return result
}
