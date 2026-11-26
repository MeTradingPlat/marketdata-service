package ingestion

import (
	"context"

	"github.com/rs/zerolog/log"
)

// FlushLiveSaves vacia liveSaveBuffer y guarda todo lo acumulado en un solo
// Save() por lote -- llamado cada pocos segundos desde cmd/api (ver
// StartLiveSaveFlushLoop). Si el lote entero falla (Postgres caido un
// momento), cada vela pasa a retryBuffer para que RetryPendingSaves la
// reintente con su propio backoff en vez de perderla.
func (s *ingestCandlesService) FlushLiveSaves(ctx context.Context) bool {
	pending := s.liveSaveBuffer.drain()
	if len(pending) == 0 {
		return true
	}
	if remainder, err := s.saveInChunks(ctx, pending); err != nil {
		log.Error().Err(err).Int("candles", len(pending)).Msg("flushing live save batch failed, buffering for retry")
		s.retryBuffer.requeue(remainder)
		return false
	}
	return true
}

// RetryPendingSaves reintenta las velas en vivo que fallaron al guardarse --
// llamado periodicamente desde cmd/api, no en el hilo de lectura del
// WebSocket (ver comentario de saveRetryBuffer sobre por que la suscripcion
// DxLink no necesita tocarse para esto). El bool de retorno le dice al
// caller si hacer backoff (ver StartSaveRetryLoop): false solo cuando habia
// algo pendiente y el intento fallo.
func (s *ingestCandlesService) RetryPendingSaves(ctx context.Context) bool {
	pending := s.retryBuffer.drain()
	if len(pending) == 0 {
		return true
	}
	if remainder, err := s.saveInChunks(ctx, pending); err != nil {
		log.Error().Err(err).Int("candles", len(pending)).Msg("retrying pending candle saves failed, will retry again")
		s.retryBuffer.requeue(remainder)
		return false
	}
	log.Info().Int("candles", len(pending)).Msg("recovered pending candle saves")
	return true
}

func (s *ingestCandlesService) PendingSaves() int {
	return s.liveSaveBuffer.size() + s.retryBuffer.size()
}
