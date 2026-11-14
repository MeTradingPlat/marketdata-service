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
