package tastytrade

import (
	"strings"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// candleKey identifica un slot ocupado (en vivo o historial) tanto para
// contabilidad de capacidad como para el registro de dispatch -- un canal
// pooled puede servir varias suscripciones distintas a la vez.
func candleKey(symbol string, tf domain.Timeframe) string {
	return symbol + "|" + string(tf)
}

// liveSymbolFromKey identifica las claves que son suscripciones en vivo
// (M1) para saber cuales resuscribir tras una reconexion -- las de
// historial (H1/D1 puntuales) ya terminaron y no necesitan restaurarse.
func liveSymbolFromKey(key string) (string, bool) {
	suffix := "|" + string(domain.M1)
	if !strings.HasSuffix(key, suffix) {
		return "", false
	}
	return strings.TrimSuffix(key, suffix), true
}
