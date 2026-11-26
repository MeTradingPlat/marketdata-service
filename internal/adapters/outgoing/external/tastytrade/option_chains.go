package tastytrade

import (
	"context"
	"strconv"
	"time"
)

// OpenInterest suma el open interest del vencimiento mensual mas cercano
// (convencion de la industria para "Open Interest" de un subyacente) --
// cadena /nested + lotes de 100 contratos por simbolo, cacheado 6h. Un
// simbolo sin opciones (o con la cadena vacia) devuelve (0, false) SIN
// error: no es un fallo del servicio, es un dato que no existe -- el
// detalle lo omite y el frontend muestra N/A (misma disciplina null-vs-0
// del resto de fundamentales).
func (g *Gateway) OpenInterest(ctx context.Context, symbol string) (float64, bool, error) {
	if value, found, err, ok := g.openInterestCache.get(symbol); ok {
		return value, found, err
	}

	expirations, err := g.optionExpirations(ctx, symbol)
	if err != nil {
		if isNotFoundError(err) {
			g.openInterestCache.put(symbol, 0, false)
			return 0, false, nil
		}
		return g.openInterestFailure(ctx, symbol, "open interest: failed to load option chain", err)
	}

	expiration, ok := frontMonthlyExpiration(expirations, time.Now())
	if !ok {
		g.openInterestCache.put(symbol, 0, false)
		return 0, false, nil
	}

	total, err := g.sumOpenInterest(ctx, expiration)
	if err != nil {
		return g.openInterestFailure(ctx, symbol, "open interest: failed to load front month quotes", err)
	}
	g.openInterestCache.put(symbol, total, total > 0)
	return total, total > 0, nil
}

// openInterestValue tolera el numero crudo o en string -- TastyTrade
// devuelve los campos numericos como strings en varias respuestas (mismo
// patrón que el parseo de eps en earnings_reports.go).
func openInterestValue(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err == nil {
			return f
		}
	}
	return 0
}
