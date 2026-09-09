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
	ch, err := p.allocator.allocate(ctx)
	if err != nil {
		return fmt.Errorf("allocating channel for %s live M1: %w", symbol, err)
	}

	p.liveMu.Lock()
	p.liveSubs[symbol] = onClosed
	p.liveTicks[symbol] = onTick
	p.liveMu.Unlock()

	_ = p.registerDispatch(symbol, domain.M1, "live", func(ev rawCandleEvent) { p.handleLiveEvent(symbol, ev) })
	ch.occupy(candleKey(symbol, domain.M1))

	if err := ch.channel.subscribeLive(symbol, domain.M1, from); err != nil {
		return fmt.Errorf("subscribing live M1 for %s: %w", symbol, err)
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

// RefreshLiveSubscriptions reenvia el Add M1 de cada simbolo ya suscrito,
// por su MISMO canal (nunca abre un canal o conexion nueva, ver
// channelAllocator.allChannels) -- un remove+add de la misma clave deja el
// streaming mudo para siempre (ver el comentario de subscribeLive), pero un
// Add repetido sobre una suscripcion que sigue viva es inofensivo: dxLink
// fusiona los "add" al mismo tema (ver el comentario de
// unsubscribeHistoryBatch) y, con FromTime distinto de cero, repite el
// historial desde ese punto antes de seguir en vivo por la misma
// suscripcion. Para una suscripcion que dejo de empujar datos en silencio
// (el caso que motivo esto, confirmado en vivo el 2026-09-04 con BSV/TW:
// canal y conexion sanos, cero eventos nuevos igual), este re-Add es la
// unica señal que le llega al servidor para reintentar sin que nosotros
// sepamos de antemano cual simbolo puntual esta mudo -- se manda a todos
// por igual, un mensaje por canal (no por simbolo), asi que el costo es
// proporcional a la cantidad de canales (~130-150), no al universo (~13k).
//
// refreshCycle alterna en dos mitades (canales pares/impares) cual grupo se
// refresca en cada llamada del ticker de 1 min -- confirmado en vivo el
// 2026-09-05: mandar los ~130-150 mensajes de TODOS los canales juntos, uno
// tras otro con solo 50ms de por medio, disparo un reenganche masivo de
// sesiones ("dispatch registration replaced" por miles en segundos) que
// termino saturando el limite de sesiones de TastyTrade minutos despues.
// Partiendo el barrido a la mitad, cada minuto manda la mitad de los
// mensajes (~65-75) y cada canal puntual se refresca cada 2 min en vez de
// cada 1 -- refreshLiveSubscriptionsFrom ya cubre exactamente esa ventana,
// asi que no queda ningun hueco sin repetir.
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
}

// handleLiveEvent detecta el cierre de una vela: mientras los eventos que
// llegan comparten el mismo timestamp, son actualizaciones de la vela en
// formacion; un timestamp MAS NUEVO significa que la anterior ya cerro. En
// los dos casos se reenvia la vela en formacion actualizada (dispatchTick)
// -- los graficos la necesitan tick a tick, no solo al cierre.
//
// Un timestamp MAS VIEJO que la vela en formacion es un caso aparte: una
// correccion tardia de una vela YA cerrada (el trade real ocurrio en ese
// minuto pero el reporte del exchange/SIP llego despues de que ya cerramos
// el minuto siguiente -- documentado, no un caso raro: "el primer trade del
// minuto nuevo puede llegar varios segundos despues del cierre formal si el
// simbolo opera poco o hay demoras tecnicas"). Antes, el simple "timestamp
// distinto" de mas abajo confundia esto con una vela nueva -- cerraba de
// golpe la vela en formacion REAL con datos a medio completar y reabria la
// vieja como si fuera la actual. Confirmado en vivo el 2026-08-27/28: el
// volumen real de un pico de un minuto tardaba varios ciclos del escaner en
// reflejarse completo rio abajo. Ahora se despacha aparte via dispatchClosed,
// SIN tocar current -- fusionada contra lastClosed[symbol] (la ultima vela
// que SI cerro de verdad) para no perder los campos que dxLink no reenvio
// (formato COMPACT: solo manda lo que cambio). Si lastClosed no tiene ese
// timestamp exacto (la correccion apunta mas atras de una vela, caso raro),
// se fusiona sobre una base vacia como antes -- mejor esfuerzo, no hay de
// donde mas sacar el resto de los campos.
func (p *CandlePool) handleLiveEvent(symbol string, ev rawCandleEvent) {
	p.lastLiveEventAtUnixNano.Store(time.Now().UnixNano())
	p.currentMu.Lock()
	prev, exists := p.current[symbol]
	if exists && ev.Timestamp.Before(prev.Timestamp) {
		base := domain.Candle{}
		if last, ok := p.lastClosed[symbol]; ok && last.Timestamp.Equal(ev.Timestamp) {
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
		// Un minuto sin ticks no deja vela -- por diseño: la vela solo se
		// cierra cuando llega un tick del minuto siguiente, y un minuto
		// muerto (sin operaciones) no genera ningun evento. Sintetizar velas
		// planas para los minutos intermedios (open=close al ultimo precio,
		// volumen 0) parecia dar continuidad al grafico, pero el usuario lo
		// rechazo en vivo el 2026-08-19: barras identicas donde no cambio
		// nada ensucian la serie -- el chart debe mostrar barras solo donde
		// hubo movimiento real (como antes de a19301a).
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

// flushFormingCandles guarda la vela EN FORMACION de cada simbolo antes de
// que StopAllLive/CloseAllConnections la descarten -- confirmado en vivo el
// 2026-09-01: ninguno de los dos guardaba la vela a medio cerrar, asi que
// se perdia en silencio cada vez que el proceso se reinicia (hoy paso 6+
// veces) o en cada ventana de mantenimiento nocturna, hasta que el barrido
// M1 la volvia a traer de TastyTrade horas despues. Reusa dispatchClosed
// (mismo camino de guardado que un cierre real, con su chequeo IsComplete)
// en vez de inventar uno nuevo -- del lado de afuera es indistinguible de
// un cierre real, solo que forzado por el apagado en vez de un tick nuevo.
// Guardar por este camino usa withWatermark=false igual que un cierre
// normal, asi que si quedo incompleta el proximo barrido M1 la reemplaza
// con la version real de TastyTrade sin conflicto.
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

// hasLiveSub dice si un simbolo ya tiene una suscripcion M1 en vivo activa.
func (p *CandlePool) hasLiveSub(symbol string) bool {
	p.liveMu.Lock()
	defer p.liveMu.Unlock()
	_, ok := p.liveSubs[symbol]
	return ok
}

// LiveSubscribed dice si el stream M1 del simbolo esta ACTUALMENTE
// registrado en el pool -- el reconciliador (cmd/api) lo usa para
// detectar las muertes silenciosas (un resubscribe fallido tras una
// reconexion deja el stream mudo sin error visible, confirmado en vivo el
// 2026-08-18 con OSRH) y resuscribirlas aunque la marca del ingestor siga
// en true.
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
