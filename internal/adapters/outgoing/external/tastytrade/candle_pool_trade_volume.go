package tastytrade

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// tradeBatchSize/tradeQuietPeriod/tradeMaxWait: mismos valores que
// FetchProfileShares (ver ese comentario para el limite de 65536 bytes por
// mensaje) -- misma forma de pedido puntual de snapshot, distinto evento.
const (
	tradeBatchSize   = 1500
	tradeQuietPeriod = 3 * time.Second
	tradeMaxWait     = 60 * time.Second
)

// FetchDayVolumes resuelve el volumen real del dia (consolidado, no el
// 40-60% que trae la suma de Candle.volume) via el evento Trade de DxLink
// -- snapshot puntual como FetchProfileShares, reusa canales de las
// conexiones ya abiertas del pool de velas.
func (p *CandlePool) FetchDayVolumes(ctx context.Context, symbols []string) map[string]int64 {
	if len(symbols) == 0 {
		return map[string]int64{}
	}

	result := make(map[string]int64)
	for i := 0; i < len(symbols); i += tradeBatchSize {
		end := min(i+tradeBatchSize, len(symbols))
		for symbol, volume := range p.fetchTradeChunk(ctx, symbols[i:end]) {
			result[symbol] = volume
		}
	}
	return result
}

func (p *CandlePool) fetchTradeChunk(ctx context.Context, symbols []string) map[string]int64 {
	ch, err := p.allocator.allocate(ctx)
	if err != nil {
		log.Error().Err(err).Int("symbols", len(symbols)).Msg("dxlink day volume fetch: failed to allocate channel")
		return map[string]int64{}
	}

	collector := newTradeCollector()
	prev := ch.channel.onTrade
	ch.channel.setOnTrade(collector.onTrade)
	defer ch.channel.setOnTrade(prev)

	if err := ch.channel.subscribeTrade(symbols); err != nil {
		log.Error().Err(err).Int("symbols", len(symbols)).Msg("dxlink day volume fetch: failed to subscribe")
		return map[string]int64{}
	}
	_ = waitForData(ctx, func() bool { return collector.settled(len(symbols), tradeQuietPeriod) }, tradeMaxWait)
	_ = ch.channel.unsubscribeTrade(symbols)
	return collector.result()
}
