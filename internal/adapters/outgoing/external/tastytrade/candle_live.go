package tastytrade

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/rs/zerolog/log"
)

// SubscribeLive es la unica suscripcion M1 de un simbolo: se abre con
// FromTime = from (para retomar exactamente donde quedo el ultimo dato
// guardado) y se queda abierta para siempre, sin un FetchHistory M1
// separado antes ni un remove/add de por medio -- ver el comentario de
// subscribeLive sobre por que esa secuencia dejaba el streaming mudo.
func (p *CandlePool) SubscribeLive(ctx context.Context, symbol string, from time.Time, onClosed func(domain.Candle), onTick func(domain.Candle)) error {
	if from.IsZero() {
		from = time.Now().Add(-5 * time.Minute)
	}
	ch, err := p.claimLiveChannel(ctx, symbol)
	if err != nil {
		return fmt.Errorf("allocating channel for %s live M1: %w", symbol, err)
	}

	p.liveMu.Lock()
	p.liveSubs[symbol] = onClosed
	p.liveTicks[symbol] = onTick
	p.liveMu.Unlock()

	_ = p.registerDispatch(symbol, domain.M1, "live", func(ev rawCandleEvent) { p.handleLiveEvent(symbol, ev) })

	if err := ch.channel.subscribeLive(symbol, domain.M1, from); err != nil {
		return fmt.Errorf("subscribing live M1 for %s: %w", symbol, err)
	}
	// subscribeLiveDayVolume va sobre el MISMO canal, sin ocupar un slot de
	// cupo nuevo (ver day_volume_live.go) -- un fallo aca no debe tumbar la
	// suscripcion M1 que ya quedo activa, solo loguea y sigue: el volumen
	// del dia tiene el batch FetchDayVolumes como respaldo.
	if err := ch.channel.subscribeLiveDayVolume(symbol); err != nil {
		log.Warn().Err(err).Str("symbol", symbol).Msg("failed to subscribe live day volume (Trade/TradeETH), falling back to batch fetch")
	}
	return nil
}

// refreshLiveSubscriptionsFrom es cuanto historial repetir en cada refresco
// -- el intervalo real entre dos refrescos DEL MISMO canal ya no es 1 min
// sino 2: RefreshLiveSubscriptions alterna la mitad de canales que toca en
// cada llamada (ver ese comentario). 3 min deja 1 min de margen sobre esos
// 2 exactos -- sin margen, un ciclo del ticker atrasado un poco (GC, carga)
// dejaria un hueco sin repetir. Si el stream seguia sano, dxLink ya mando
// esas mismas velas hace un momento y el upsert por timestamp las pisa sin
// duplicar nada.
const refreshLiveSubscriptionsFrom = 3 * time.Minute

// RefreshLiveSubscriptions reenvia el Add M1 de cada simbolo por su MISMO canal, sin remove+add ni abrir canal/conexion nueva.
//
// refreshCycle alterna en dos mitades (canales pares/impares) cual grupo se refresca en cada llamada al ticker de 1 min.
//
// refreshChannelStagger espacia el envio DENTRO de cada mitad -- TastyTrade
// publica un tope de 10,000 "subscription changes"/minuto para dxLink, pero
// no documenta si eso cuenta por mensaje o por simbolo dentro del Add, ni si
// hay ademas un limite de rafaga por segundo aparte del limite por minuto.
// Con 50ms entre canales, cada mitad (~65-75 canales) tarda unos ~3.5s en
// el caso real y ~8s en el techo teorico (320 canales, 40 conexiones x 8
// canales / 2), con margen de sobra dentro del minuto entre una vuelta del
// ticker y la siguiente.
const refreshChannelStagger = 50 * time.Millisecond

func (p *CandlePool) RefreshLiveSubscriptions(ctx context.Context) {
	all := p.allocator.allChannels()
	half := p.refreshCycle.Add(1) % 2
	channels := make([]*pooledChannel, 0, (len(all)+1)/2)
	for i, ch := range all {
		if uint64(i)%2 == half {
			channels = append(channels, ch)
		}
	}

	from := time.Now().Add(-refreshLiveSubscriptionsFrom)
	refreshed := 0
	for i, ch := range channels {
		if i > 0 {
			select {
			case <-time.After(refreshChannelStagger):
			case <-ctx.Done():
				return
			}
		}
		symbols := ch.liveSymbols()
		if len(symbols) == 0 {
			continue
		}
		froms := make(map[string]time.Time, len(symbols))
		for _, sym := range symbols {
			froms[sym] = from
		}
		if err := ch.channel.subscribeHistoryBatch(symbols, domain.M1, froms); err != nil {
			log.Warn().Err(err).Int("symbols", len(symbols)).Msg("live subscription refresh failed for a channel")
			continue
		}
		refreshed += len(symbols)
	}
	log.Info().Int("channels", len(channels)).Int("symbols", refreshed).Msg("live subscriptions refreshed")
	if orphans := p.orphanedLiveSymbols(); len(orphans) > 0 {
		sample := orphans
		if len(sample) > 5 {
			sample = sample[:5]
		}
		log.Warn().Int("count", len(orphans)).Strs("sample", sample).Msg("live symbols outside any pooled channel")
	}
}

// handleLiveEvent distingue actualizacion de la vela en formacion, cierre organico y correccion tardia de una vela ya cerrada (via dispatchClosed, fusionada contra lastClosed).
func (p *CandlePool) handleLiveEvent(symbol string, ev rawCandleEvent) {
	now := time.Now()
	p.lastLiveEventAtUnixNano.Store(now.UnixNano())
	p.noteLiveEvent(symbol, now)
	p.currentMu.Lock()
	prev, exists := p.current[symbol]
	last, hasLast := p.lastClosed[symbol]
	// isLateCorrection cubre los dos caminos por los que un minuto puede
	// haber quedado "ya cerrado" antes de que este evento llegara: el cierre
	// organico de arriba (existe un "current" mas nuevo) y el cierre por
	// tiempo de CloseElapsedForming (current ya se borro porque nadie mando
	// un tick a tiempo, pero lastClosed s? tiene ese minuto). En los dos
	// casos hay que fusionar contra lastClosed, no arrancar una vela "nueva"
	// para un minuto que el sistema ya dio por cerrado y publico.
	isLateCorrection := (exists && ev.Timestamp.Before(prev.Timestamp)) ||
		(!exists && hasLast && !ev.Timestamp.After(last.Timestamp))
	if isLateCorrection {
		base := domain.Candle{}
		if hasLast && last.Timestamp.Equal(ev.Timestamp) {
			base = last
		}
		corrected := mergeCandle(base, ev, symbol, domain.M1)
		unchanged := candleValuesEqual(corrected, base)
		p.lastClosed[symbol] = corrected
		p.currentMu.Unlock()
		// unchanged descarta el replay del refresco periodico de suscripcion
		// (RefreshLiveSubscriptions pide el historial de los ultimos 2 min por
		// canal cada minuto para TODO simbolo en vivo, no solo los que de
		// verdad tuvieron una correccion) -- sin este filtro, cada vuelta
		// reencolaba en liveSaveBuffer las mismas velas ya guardadas de los
		// ~13k simbolos en vivo, multiplicando el tamaño del lote de guardado
		// sin ningun dato nuevo real.
		if !unchanged {
			p.dispatchClosed(symbol, corrected)
		}
		return
	}

	var forming domain.Candle
	if exists && !prev.Timestamp.Equal(ev.Timestamp) {
		// Cierre organico: llego un tick de un minuto mas nuevo; no se sintetizan velas planas para los minutos intermedios.
		closed := prev
		p.current[symbol] = mergeCandle(domain.Candle{}, ev, symbol, domain.M1)
		p.lastClosed[symbol] = closed
		forming = p.current[symbol]
		p.currentMu.Unlock()
		p.dispatchClosed(symbol, closed)
		p.dispatchTick(symbol, forming)
		return
	}
	forming = mergeCandle(prev, ev, symbol, domain.M1)
	p.current[symbol] = forming
	p.currentMu.Unlock()
	p.dispatchTick(symbol, forming)
}

// closeElapsedGracePeriod: margen sobre el fin exacto del minuto antes de
// forzar el cierre por tiempo -- deja que un tick que ya esta en camino (la
// latencia normal de red/procesamiento) todavia alcance a cerrar la vela
// por la via organica de arriba antes de que el sweep la fuerce con los
// datos que tenia hasta ese momento. Mismo margen que ya usa
// signal-processing-service del otro lado (_BAR_CLOSE_BUFFER_SECONDS) y que
// RangeAggregated (RecentCache) para el mismo motivo.
const closeElapsedGracePeriod = 1 * time.Second

// CloseElapsedForming cierra por RELOJ cualquier vela M1 en formacion cuyo
// minuto ya termino, sin esperar un tick del minuto siguiente -- sin esto,
// un simbolo que se queda sin ticks justo en el cierre (silencio real de
// mercado, no una suscripcion muerta) deja esa vela "en formacion" para
// siempre hasta el proximo trade, que en un simbolo poco liquido puede
// tardar minutos u horas (confirmado como el patron esperado: cierre por
// tiempo de pared, no por llegada del proximo dato). Se llama
// periodicamente desde un ticker de 1s (ver cmd/api) -- barata: solo itera
// el mapa de velas en formacion, sin ningun I/O de red de por medio.
func (p *CandlePool) CloseElapsedForming(now time.Time) {
	type closedEntry struct {
		symbol string
		candle domain.Candle
	}
	var toClose []closedEntry

	p.currentMu.Lock()
	for symbol, c := range p.current {
		if !now.Before(c.Timestamp.Add(time.Minute + closeElapsedGracePeriod)) {
			toClose = append(toClose, closedEntry{symbol: symbol, candle: c})
			delete(p.current, symbol)
			p.lastClosed[symbol] = c
		}
	}
	p.currentMu.Unlock()

	for _, entry := range toClose {
		p.dispatchClosed(entry.symbol, entry.candle)
	}
}

// flushFormingCandles guarda la vela en formacion de cada simbolo antes de que StopAllLive/CloseAllConnections la descarten.
func (p *CandlePool) flushFormingCandles() {
	p.currentMu.Lock()
	pending := make(map[string]domain.Candle, len(p.current))
	for symbol, c := range p.current {
		pending[symbol] = c
	}
	p.currentMu.Unlock()

	for symbol, c := range pending {
		p.dispatchClosed(symbol, c)
	}
}

func (p *CandlePool) dispatchClosed(symbol string, c domain.Candle) {
	if !c.IsComplete() {
		return
	}
	p.liveMu.Lock()
	cb := p.liveSubs[symbol]
	p.liveMu.Unlock()
	if cb != nil {
		cb(c)
	}
}

// dispatchTick reenvia la vela en formacion tras cada tick -- el suscriptor
// (StreamLive -> Broadcaster) descarta si su canal va lleno, asi un cliente
// lento jamas frena el merge de ticks de la conexion DxLink. El filtro es
// SOLO "tiene cierre": un tick parcial (minuto sin trades, primer evento
// del periodo con OHLC incompleto) se dibuja plano en el grafico y el
// siguiente tick lo completa -- descartarlo por IsComplete dejaba huecos en
// la serie en vivo (minutos enteros saltados).
func (p *CandlePool) dispatchTick(symbol string, c domain.Candle) {
	if c.Close == 0 {
		return
	}
	p.liveMu.Lock()
	cb := p.liveTicks[symbol]
	p.liveMu.Unlock()
	if cb != nil {
		cb(c)
	}
}

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
}

func (p *CandlePool) noteLiveEvent(symbol string, now time.Time) {
	p.activityMu.Lock()
	p.ensureActivityMaps()
	p.lastEventAt[symbol] = now
	p.activityMu.Unlock()
}
