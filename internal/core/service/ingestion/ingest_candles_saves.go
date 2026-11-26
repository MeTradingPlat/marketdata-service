package ingestion

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/rs/zerolog/log"
)

func (s *ingestCandlesService) flushBufferToRetry(ctx context.Context, drain func() []domain.Candle, failMsg, okMsg string) bool {
	pending := drain()
	if len(pending) == 0 {
		return true
	}
	if remainder, err := s.saveInChunks(ctx, pending); err != nil {
		log.Error().Err(err).Int("candles", len(pending)).Msg(failMsg)
		s.retryBuffer.requeue(remainder)
		return false
	}
	if okMsg != "" {
		log.Info().Int("candles", len(pending)).Msg(okMsg)
	}
	return true
}

// FlushLiveSaves vacia liveSaveBuffer y guarda todo lo acumulado en un solo
// Save() por lote -- llamado cada pocos segundos desde cmd/api (ver
// StartLiveSaveFlushLoop). Si el lote entero falla (Postgres caido un
// momento), cada vela pasa a retryBuffer para que RetryPendingSaves la
// reintente con su propio backoff en vez de perderla.
func (s *ingestCandlesService) FlushLiveSaves(ctx context.Context) bool {
	return s.flushBufferToRetry(ctx, s.liveSaveBuffer.drain, "flushing live save batch failed, buffering for retry", "")
}

// RetryPendingSaves reintenta las velas en vivo que fallaron al guardarse --
// llamado periodicamente desde cmd/api, no en el hilo de lectura del
// WebSocket (ver comentario de saveRetryBuffer sobre por que la suscripcion
// DxLink no necesita tocarse para esto). El bool de retorno le dice al
// caller si hacer backoff (ver StartSaveRetryLoop): false solo cuando habia
// algo pendiente y el intento fallo.
func (s *ingestCandlesService) RetryPendingSaves(ctx context.Context) bool {
	return s.flushBufferToRetry(ctx, s.retryBuffer.drain, "retrying pending candle saves failed, will retry again", "recovered pending candle saves")
}

func (s *ingestCandlesService) PendingSaves() int {
	return s.liveSaveBuffer.size() + s.retryBuffer.size()
}
