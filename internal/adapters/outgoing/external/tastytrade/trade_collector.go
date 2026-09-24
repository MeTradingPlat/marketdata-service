package tastytrade

import (
	"sync"
	"time"
)

// tradeCollector acumula dayVolume por simbolo de un lote -- mismo patron
// que profileCollector: dxFeed manda el snapshot de cada simbolo en
// rafagas separadas en el tiempo, no todo de una.
type tradeCollector struct {
	mu         sync.Mutex
	dayVolumes map[string]float64
	received   int
	lastUpdate time.Time
}

func newTradeCollector() *tradeCollector {
	return &tradeCollector{dayVolumes: make(map[string]float64)}
}

func (c *tradeCollector) onTrade(ev rawTradeEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ev.DayVolume == nil {
		return
	}
	if _, exists := c.dayVolumes[ev.Symbol]; !exists {
		c.received++
		c.lastUpdate = time.Now()
	}
	c.dayVolumes[ev.Symbol] = *ev.DayVolume
}

// settled: terminado cuando ya llegaron todos los simbolos esperados con
// dayVolume, o paso el periodo de silencio sin NINGUN simbolo nuevo (los
// trades repetidos de un simbolo ya visto no cuentan: con el mercado abierto
// siempre llega alguno y el lote esperaba el maximo de 60 s, ~7 min por
// ronda de 13k simbolos, confirmado en vivo el 2026-09-24) -- un simbolo sin
// ningun trade hoy (illiquido, o sin sesion) genuinamente no lo trae, asi
// que "todos" nunca se cumple en la practica para un lote mixto.
func (c *tradeCollector) settled(expected int, quietPeriod time.Duration) bool {
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

func (c *tradeCollector) result() map[string]int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[string]int64, len(c.dayVolumes))
	for symbol, v := range c.dayVolumes {
		result[symbol] = int64(v)
	}
	return result
}
