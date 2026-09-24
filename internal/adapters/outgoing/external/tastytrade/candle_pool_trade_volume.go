package tastytrade

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// tradeBatchSize: mismo valor que FetchProfileShares (ver ese comentario
// para el limite de 65536 bytes por mensaje) -- misma forma de pedido
// puntual de snapshot, distinto evento.
const (
	tradeBatchSize = 1500
	// tradeFetchConcurrency: lotes en paralelo, cada uno en su propio canal
	// -- con 13k simbolos, 9 lotes en serie tardaban ~7 min por ronda.
	tradeFetchConcurrency = 3
)

type tradePass struct {
	quiet   time.Duration
	maxWait time.Duration
}

// tradePasses: la primera pasada es rapida y trae a casi todos; la segunda
// reintenta SOLO a los que faltaron con mas paciencia -- una sola pasada
// rapida dejaba ~2900 simbolos (algunos muy activos) sin refrescar, y una
// sola pasada paciente tardaba ~7 min (confirmado en vivo el 2026-09-24).
var tradePasses = []tradePass{
	{quiet: 12 * time.Second, maxWait: 30 * time.Second},
	{quiet: 20 * time.Second, maxWait: 60 * time.Second},
}

// FetchDayVolumes resuelve el volumen real del dia (consolidado, no el
// 40-60% que trae la suma de Candle.volume) via el evento Trade de DxLink
// -- snapshot puntual como FetchProfileShares, reusa canales de las
// conexiones ya abiertas del pool de velas.
func (p *CandlePool) FetchDayVolumes(ctx context.Context, symbols []string) map[string]int64 {
	result := make(map[string]int64)
	pending := symbols
	for _, pass := range tradePasses {
		if len(pending) == 0 || ctx.Err() != nil {
			break
		}
		for symbol, volume := range p.fetchTradeBatches(ctx, pending, pass) {
			result[symbol] = volume
		}
		pending = unresolvedSymbols(pending, result)
	}
	return result
}

func unresolvedSymbols(symbols []string, resolved map[string]int64) []string {
	var missing []string
	for _, symbol := range symbols {
		if _, ok := resolved[symbol]; !ok {
			missing = append(missing, symbol)
		}
	}
	return missing
}

func (p *CandlePool) fetchTradeBatches(ctx context.Context, symbols []string, pass tradePass) map[string]int64 {
	result := make(map[string]int64)
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, tradeFetchConcurrency)
	for i := 0; i < len(symbols); i += tradeBatchSize {
		chunk := symbols[i:min(i+tradeBatchSize, len(symbols))]
		slots <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-slots; wg.Done() }()
			volumes := p.fetchTradeChunk(ctx, chunk, pass)
			mu.Lock()
			for symbol, volume := range volumes {
				result[symbol] = volume
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	return result
}

func (p *CandlePool) fetchTradeChunk(ctx context.Context, symbols []string, pass tradePass) map[string]int64 {
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
	_ = waitForData(ctx, func() bool { return collector.settled(len(symbols), pass.quiet) }, pass.maxWait)
	_ = ch.channel.unsubscribeTrade(symbols)
	return collector.result()
}
