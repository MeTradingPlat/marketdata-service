package tastytrade

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// LiveSubscribed dice si el stream M1 del simbolo esta ACTUALMENTE registrado en el pool.
func (p *CandlePool) LiveSubscribed(symbol string) bool {
	p.liveMu.Lock()
	defer p.liveMu.Unlock()
	_, ok := p.liveSubs[symbol]
	return ok
}

// LiveSubscribedCount es cuantos simbolos tienen streaming M1 registrado
// ahora mismo -- LiveDataWatchdog solo tiene sentido revisar el silencio si
// hay algo que deberia estar sonando.
func (p *CandlePool) LiveSubscribedCount() int {
	p.liveMu.Lock()
	defer p.liveMu.Unlock()
	return len(p.liveSubs)
}

// LastLiveEventAge es hace cuanto no llega NINGUNA vela en vivo de NINGUN
// simbolo -- ver el comentario de lastLiveEventAtUnixNano. Cero significa
// "todavia no llego la primera desde que arranco el proceso".
func (p *CandlePool) LastLiveEventAge() time.Duration {
	last := p.lastLiveEventAtUnixNano.Load()
	if last == 0 {
		return 0
	}
	return time.Since(time.Unix(0, last))
}

// CurrentCandle devuelve la vela M1 en formacion ahora mismo -- precio y
// volumen mas frescos que cualquier vela ya cerrada en la BD, se actualiza
// en cada tick sin esperar a que cierre el minuto.
func (p *CandlePool) CurrentCandle(symbol string) (domain.Candle, bool) {
	p.currentMu.Lock()
	defer p.currentMu.Unlock()
	c, ok := p.current[symbol]
	return c, ok
}

func (p *CandlePool) ensureActivityMaps() {
	if p.lastEventAt == nil {
		p.lastEventAt = make(map[string]time.Time)
	}
	if p.pendingRevive == nil {
		p.pendingRevive = make(map[string]reviveState)
	}
	if p.orphanSince == nil {
		p.orphanSince = make(map[string]time.Time)
	}
}

func (p *CandlePool) noteLiveEvent(symbol string, now time.Time) {
	p.activityMu.Lock()
	p.ensureActivityMaps()
	p.lastEventAt[symbol] = now
	p.activityMu.Unlock()
}
