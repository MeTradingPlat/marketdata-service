package tastytrade

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// profileBatchSize: un solo mensaje FEED_SUBSCRIPTION con 2000 items pesa
// ~70KB, por encima del limite duro de dxFeed de 65536 bytes (confirmado en
// vivo el 2026-09-04: "bytes":70250 -> INVALID_MESSAGE -> reconnect forzado
// -> ese mismo reconnect fue el que disparo un storm de "sessions exceeded"
// dos minutos despues). subscribeProfile no trocea por tamaño, solo el
// llamador via este batch size -- 1500 items da ~53KB, con margen bajo
// dxLinkWarnMessageBytes incluso con simbolos mas largos que el promedio.
const (
	profileBatchSize   = 1500
	profileQuietPeriod = 3 * time.Second
	profileMaxWait     = 60 * time.Second
)

// FetchProfileShares resuelve sharesOutstanding en vivo via el evento
// Profile de DxLink -- snapshot puntual (no suscripcion persistente:
// "shares" cambia por corporate action, no tick a tick). Reusa canales de
// las conexiones YA abiertas del pool (sin occupancy de velas: el uso es
// transitorio y no compite por la capacidad de 100 simbolos/canal de las
// velas) -- abrir una conexion propia no sirve: TastyTrade limita las
// sesiones concurrentes y el pool de velas ya llega al tope (confirmado en
// vivo: "user sessions has exceeded the configured limit" al intentarlo).
func (p *CandlePool) FetchProfileShares(ctx context.Context, symbols []string) map[string]int64 {
	if len(symbols) == 0 {
		return map[string]int64{}
	}

	result := make(map[string]int64)
	for i := 0; i < len(symbols); i += profileBatchSize {
		end := min(i+profileBatchSize, len(symbols))
		for symbol, shares := range p.fetchProfileChunk(ctx, symbols[i:end]) {
			result[symbol] = shares
		}
	}
	return result
}

func (p *CandlePool) fetchProfileChunk(ctx context.Context, symbols []string) map[string]int64 {
	ch, err := p.allocator.allocate(ctx)
	if err != nil {
		log.Error().Err(err).Int("symbols", len(symbols)).Msg("dxlink profile fetch: failed to allocate channel")
		return map[string]int64{}
	}

	collector := newProfileCollector()
	prev := ch.channel.onProfile
	ch.channel.setOnProfile(collector.onProfile)
	defer ch.channel.setOnProfile(prev)

	if err := ch.channel.subscribeProfile(symbols); err != nil {
		log.Error().Err(err).Int("symbols", len(symbols)).Msg("dxlink profile fetch: failed to subscribe")
		return map[string]int64{}
	}
	_ = waitForData(ctx, func() bool { return collector.settled(len(symbols), profileQuietPeriod) }, profileMaxWait)
	_ = ch.channel.unsubscribeProfile(symbols)
	return collector.result()
}
