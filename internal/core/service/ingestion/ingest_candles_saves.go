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

const maxConsecutiveSpoolFailures = 3

// RetryPendingSaves reintenta las velas en vivo que fallaron al guardarse --
// llamado periodicamente desde cmd/api, no en el hilo de lectura del
// WebSocket (ver comentario de saveRetryBuffer sobre por que la suscripcion
// DxLink no necesita tocarse para esto). El bool de retorno le dice al
// caller si hacer backoff (ver StartSaveRetryLoop): false solo cuando habia
// algo pendiente y el intento fallo.
func (s *ingestCandlesService) RetryPendingSaves(ctx context.Context) bool {
	memOK := s.flushBufferToRetry(ctx, s.retryBuffer.drain, "retrying pending candle saves failed, will retry again", "recovered pending candle saves")
	if s.spool == nil {
		return memOK
	}
	spoolOK := s.retrySpoolBatches(ctx)
	return memOK && spoolOK
}

func (s *ingestCandlesService) retrySpoolBatches(ctx context.Context) bool {
	ids, err := s.spool.Pending()
	if err != nil {
		log.Error().Err(err).Msg("listing spooled candle batches failed")
		return false
	}
	allOK := true
	consecutiveFailures := 0
	for _, id := range ids {
		if ctx.Err() != nil {
			return false
		}
		if s.retryOneSpoolBatch(ctx, id) {
			consecutiveFailures = 0
			continue
		}
		allOK = false
		consecutiveFailures++
		if consecutiveFailures >= maxConsecutiveSpoolFailures {
			return false
		}
	}
	return allOK
}

func (s *ingestCandlesService) retryOneSpoolBatch(ctx context.Context, id string) bool {
	candles, err := s.spool.Read(id)
	if err != nil {
		log.Error().Err(err).Str("batch", id).Msg("reading spooled candle batch failed")
		return true
	}
	if len(candles) > 0 {
		if _, err := s.saveInChunks(ctx, candles); err != nil {
			log.Error().Err(err).Str("batch", id).Int("candles", len(candles)).Msg("retrying spooled candle batch failed")
			return false
		}
	}
	if err := s.spool.Remove(id); err != nil {
		log.Warn().Err(err).Str("batch", id).Msg("removing spooled candle batch failed")
	}
	log.Info().Str("batch", id).Int("candles", len(candles)).Msg("recovered spooled candle batch")
	return true
}

func (s *ingestCandlesService) SpoolPendingSaves() {
	if s.spool == nil {
		return
	}
	pending := append(s.retryBuffer.drain(), s.liveSaveBuffer.drain()...)
	if len(pending) == 0 {
		return
	}
	if err := s.spool.Write(pending); err != nil {
		log.Error().Err(err).Int("candles", len(pending)).Msg("spooling pending candles on shutdown failed")
		s.retryBuffer.restore(pending)
	}
}

func (s *ingestCandlesService) PendingSaves() int {
	return s.liveSaveBuffer.size() + s.retryBuffer.size()
}
