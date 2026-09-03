package ingestion

import "github.com/MeTradingPlat/marketdata-service/internal/core/domain"

// publishSnapshot manda la sesion intradia acumulada del simbolo a quien
// este suscripto a /ws/snapshot -- se llama justo despues de
// RecordClosedCandle, asi que ya incluye la vela que acaba de cerrar. El
// precio/volumen "actual" es el de esta misma vela: es la fuente mas fresca
// que hay en este punto, sin pagar otra consulta (mismo dato que
// GetSnapshot usaria como fallback si el simbolo no tuviera vela en
// formacion en el gateway).
func (s *ingestCandlesService) publishSnapshot(c domain.Candle) {
	if s.snapshotBroadcaster == nil {
		return
	}
	snap := s.tracker.SnapshotBatch([]string{c.Symbol})[c.Symbol]
	snap.Symbol = c.Symbol
	snap.AsOf = c.Timestamp
	snap.CurrentPrice = c.Close
	snap.CurrentVolume = c.Volume
	s.snapshotBroadcaster.Publish(c.Symbol, snap)
}
