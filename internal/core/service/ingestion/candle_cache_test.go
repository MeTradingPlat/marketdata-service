package ingestion

import (
	"strconv"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func newTestCandleCache() *candleCache {
	return &candleCache{entries: make(map[string]candleCacheEntry)}
}

func seriesOf(n int) []domain.Candle {
	return make([]domain.Candle, n)
}

func TestCandleCache_TotalDeVelasNuncaSuperaElTope(t *testing.T) {
	c := newTestCandleCache()
	perEntry := candleCacheMaxCandles / 10

	for i := 0; i < 25; i++ {
		c.put("k"+strconv.Itoa(i), perEntry, seriesOf(perEntry))
		if c.total > candleCacheMaxCandles {
			t.Fatalf("total=%d supera el tope %d tras %d puts", c.total, candleCacheMaxCandles, i+1)
		}
	}
	if c.total == 0 || len(c.entries) == 0 {
		t.Fatal("el cache no deberia quedar vacio")
	}
}

func TestCandleCache_UnaSerieEnormeNoSeCachea(t *testing.T) {
	c := newTestCandleCache()

	c.put("gigante", candleCacheMaxCandles, seriesOf(candleCacheMaxCandles))

	if _, ok := c.get("gigante", 1); ok || c.total != 0 {
		t.Fatalf("una serie enorme no debe cachearse (total=%d)", c.total)
	}
}

func TestCandleCache_ReemplazarUnaLlaveNoDuplicaElConteo(t *testing.T) {
	c := newTestCandleCache()

	c.put("AAPL", 100, seriesOf(100))
	c.put("AAPL", 100, seriesOf(40))

	if c.total != 40 || len(c.entries) != 1 {
		t.Fatalf("total=%d entries=%d, want 40 y 1", c.total, len(c.entries))
	}
}

func TestCandleCache_UnaEntradaVencidaDescuentaSuTotalAlLeerse(t *testing.T) {
	c := newTestCandleCache()
	c.put("AAPL", 100, seriesOf(100))
	entry := c.entries["AAPL"]
	entry.expires = entry.expires.Add(-2 * candleCacheTTL)
	c.entries["AAPL"] = entry

	if _, ok := c.get("AAPL", 100); ok || c.total != 0 {
		t.Fatalf("una entrada vencida debe desaparecer y descontar su total (total=%d)", c.total)
	}
}

func TestCandleCache_UnaSerieGuardadaSirveLosPedidosMasChicosConSuCola(t *testing.T) {
	c := newTestCandleCache()
	series := make([]domain.Candle, 300)
	for i := range series {
		series[i].Volume = int64(i)
	}
	c.put("AAPL", 300, series)

	got, ok := c.get("AAPL", 151)

	if !ok || len(got) != 151 || got[0].Volume != 149 || got[150].Volume != 299 {
		t.Fatalf("ok=%v len=%d, want 151 velas de la cola (149..299)", ok, len(got))
	}
}

func TestCandleCache_UnPedidoConMasBarrasQueLasGuardadasEsMiss(t *testing.T) {
	c := newTestCandleCache()
	c.put("AAPL", 151, seriesOf(151))

	if _, ok := c.get("AAPL", 300); ok {
		t.Fatal("pedir mas barras que las guardadas debe ir a la BD")
	}
}

func TestCandleCache_UnaSerieMasChicaNoPisaUnaMasGrandeVigente(t *testing.T) {
	c := newTestCandleCache()
	c.put("AAPL", 300, seriesOf(300))

	c.put("AAPL", 151, seriesOf(151))

	if got, ok := c.get("AAPL", 300); !ok || len(got) != 300 {
		t.Fatalf("la serie grande debe seguir guardada, ok=%v len=%d", ok, len(got))
	}
}

func TestCandleCache_ElBarridoLiberaLasVencidasSinNecesitarUnaLectura(t *testing.T) {
	c := newTestCandleCache()
	c.put("AAPL", 100, seriesOf(100))
	entry := c.entries["AAPL"]
	entry.expires = entry.expires.Add(-2 * candleCacheTTL)
	c.entries["AAPL"] = entry
	c.lastSweep = c.lastSweep.Add(-2 * candleCacheSweepInterval)

	c.put("MSFT", 50, seriesOf(50))

	if _, exists := c.entries["AAPL"]; exists || c.total != 50 {
		t.Fatalf("la vencida debio barrerse (total=%d)", c.total)
	}
}
