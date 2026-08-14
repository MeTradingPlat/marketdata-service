package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/rs/zerolog/log"
)

const (
	streamLiveMaxAttempts = 3
	streamLiveRetryDelay  = 10 * time.Second
)

// startLiveWithRetry existe porque, a diferencia de un corte de conexion ya
// establecida (que se autosana solo via reconexion + FromTime), un fallo en
// el intento INICIAL de StreamLive no tiene ningun mecanismo que lo
// reintente -- el simbolo se quedaria mudo hasta el proximo reinicio del
// servicio. Un fallo puntual de DxLink justo en ese instante es el
// candidato mas probable, y reintentar es barato: FromTime retoma desde el
// watermark sin perder nada aunque tarde un par de intentos en prender.
func startLiveWithRetry(ctx context.Context, ingest in.IngestCandlesService, symbol string) {
	var err error
	for attempt := 1; attempt <= streamLiveMaxAttempts; attempt++ {
		if err = ingest.StreamLive(ctx, symbol); err == nil {
			return
		}
		log.Error().Err(err).Str("symbol", symbol).Int("attempt", attempt).
			Msg("failed to start live candle stream")
		if attempt < streamLiveMaxAttempts {
			time.Sleep(streamLiveRetryDelay)
		}
	}
	log.Error().Err(err).Str("symbol", symbol).Int("attempts", streamLiveMaxAttempts).
		Msg("giving up on live candle stream after all retries failed")
}
