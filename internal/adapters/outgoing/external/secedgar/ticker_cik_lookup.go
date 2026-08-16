package secedgar

import (
	"context"
	"sync"
	"time"
)

const tickersURL = "https://www.sec.gov/files/company_tickers.json"

// tickerCikFailureCooldown evita que un solo 429/HTML-en-vez-de-JSON de la
// SEC haga que cada simbolo de un batch reintente la misma llamada fallida
// de inmediato, multiplicando el trafico justo cuando la SEC ya esta
// limitando (confirmado en Java via threaddump: 7 fallos identicos en
// ~200ms).
const tickerCikFailureCooldown = 5 * time.Minute

// TickerCikLookup resuelve simbolo->CIK contra el mapa publico de la SEC,
// cacheado en disco una vez por dia y reutilizado en memoria durante el
// resto del proceso.
type TickerCikLookup struct {
	cacheDir string

	mu            sync.Mutex
	tickerToCik   map[string]int
	lastFailureAt time.Time
}

func NewTickerCikLookup(cacheDir string) *TickerCikLookup {
	return &TickerCikLookup{cacheDir: cacheDir}
}

func (l *TickerCikLookup) TickerToCikMap(ctx context.Context) map[string]int {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.tickerToCik != nil {
		return l.tickerToCik
	}
	if !l.lastFailureAt.IsZero() && time.Now().Before(l.lastFailureAt.Add(tickerCikFailureCooldown)) {
		return map[string]int{}
	}

	m, err := l.loadCachedOrDownload(ctx)
	if err != nil {
		l.lastFailureAt = time.Now()
		return map[string]int{}
	}
	l.tickerToCik = m
	return m
}
