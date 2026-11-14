package ingestion

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// StreamLive arranca la unica suscripcion M1 del simbolo desde el ultimo
// watermark guardado -- retoma exactamente donde quedo el ultimo dato (sin
// watermark todavia, el gateway pide profundidad maxima) en vez de un
// Backfill(M1) puntual seguido de un StreamLive en vivo aparte: esa
// secuencia (desuscribir y resuscribir la misma clave casi al mismo tiempo)
// dejaba el streaming mudo para siempre sin ningun error. SubscribeLiveCandles
// solo invoca el callback con velas ya cerradas -- el merge de ticks
// parciales es responsabilidad del adaptador, no de este use case.
func (s *ingestCandlesService) StreamLive(ctx context.Context, symbol string) error {
	newest, err := s.repo.GetWatermark(ctx, symbol, domain.M1)
	if err != nil {
		return fmt.Errorf("checking watermark for %s M1: %w", symbol, err)
	}
	var from time.Time
	if newest != nil {
		from = *newest
	}
	if err := s.gateway.SubscribeLiveCandles(ctx, symbol, from, func(c domain.Candle) {
		// El guardado en si no corre aca -- se junta en liveSaveBuffer y se
		// vacia por lote cada pocos segundos (ver FlushLiveSaves), asi que
		// esta closure nunca toca Postgres directamente en el camino
		// caliente de cada vela cerrada.
		s.liveSaveBuffer.add(c)
		s.tracker.RecordClosedCandle(c)
		// Ademas de Postgres: una lectura de candles del minuto que acaba de
		// cerrar no debe depender de que la fila ya sea visible en la BD --
		// confirmado en vivo el 2026-08-27 con EMAT (RELATIVE_VOLUME M5): el
		// volumen real de una vela recien cerrada tardo varios ciclos del
		// escaner en reflejarse completo rio abajo.
		s.recentCache.Put(c, true)
		s.broadcaster.Publish(c.Symbol, c)
		s.publishSnapshot(c)
	}, func(c domain.Candle) {
		// La vela M1 en formacion tras cada tick -- el WS de /ws/candles la
		// usa para dibujar la vela actual (M1 directo, H1/D1 por
		// agregacion). Sin publicador en el Broadcaster no habria forma de
		// que una sesion WS vea la vela a medio formar: solo se publicaba
		// al cerrar.
		s.recentCache.Put(c, false)
		s.broadcaster.Publish(c.Symbol, c)
	}); err != nil {
		s.setLive(symbol, false)
		return err
	}
	s.setLive(symbol, true)
	return nil
}

func (s *ingestCandlesService) setLive(symbol string, live bool) {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	s.live[symbol] = live
	s.attempted[symbol] = true
}

// IsLive dice si el stream en vivo del simbolo arranco.
func (s *ingestCandlesService) IsLive(symbol string) bool {
	s.liveMu.RLock()
	defer s.liveMu.RUnlock()
	return s.live[symbol]
}

// IsAttempted distingue "el rollout lo intento y fallo" de "nadie lo ha
// intentado todavia" -- el reconciliador (cmd/api) solo reintenta los
// intentados-fallidos; reintentar los nunca-intentados en el primer tick
// (5 min tras boot, cuando el ciclo recien va por D1/beta) re-registra los
// dispatch entries del rollout en curso y lo frena (confirmado en vivo el
// 2026-08-18 18:07-18:20: el ciclo quedo atascado 15+ min sin abrir).
func (s *ingestCandlesService) IsAttempted(symbol string) bool {
	s.liveMu.RLock()
	defer s.liveMu.RUnlock()
	return s.attempted[symbol]
}
