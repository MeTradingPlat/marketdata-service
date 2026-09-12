package ingestion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/ingestion"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
)

func TestGetCandles(t *testing.T) {
	repo := &fakeRepo{getResult: []domain.Candle{{Symbol: "AAPL"}}}
	svc := ingestion.NewGetCandlesService(repo, livecandles.NewDefaultRecentCache())

	got, err := svc.GetCandles(context.Background(), "AAPL", domain.D1, 10, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d candles, want 1", len(got))
	}
}

// Regression: una fila de un minuto recien cerrado podia no ser visible
// todavia en Postgres cuando alguien pedia "M1 hasta ahora" -- confirmado
// en vivo el 2026-08-27 con EMAT. GetCandles(M1, before=nil) debe traer lo
// mas reciente de RecentCache, no solo lo que la BD ya tenga.
func TestGetCandles_M1UpToNow_UsesRecentCacheForTheTail(t *testing.T) {
	// Alineados al minuto (:00 segundos) -- como cualquier vela M1 real.
	minute := time.Date(2026, 8, 27, 18, 16, 0, 0, time.UTC)
	older := domain.Candle{Symbol: "EMAT", Timeframe: domain.M1, Timestamp: minute}
	repo := &fakeRepo{getResult: []domain.Candle{older}}

	cache := livecandles.NewDefaultRecentCache()
	freshest := domain.Candle{Symbol: "EMAT", Timeframe: domain.M1, Timestamp: minute.Add(time.Minute), Volume: 27165}
	cache.Put(freshest, true)

	svc := ingestion.NewGetCandlesService(repo, cache)

	got, err := svc.GetCandles(context.Background(), "EMAT", domain.M1, 10, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d candles, want 2 (db + cache)", len(got))
	}
	if got[len(got)-1].Volume != 27165 {
		t.Errorf("expected the freshest candle from the cache, got volume %d", got[len(got)-1].Volume)
	}
}

// Regression: el caso real de EMAT (2026-08-27, RELATIVE_VOLUME en M5) --
// una consulta de un timeframe DERIVADO "hasta ahora" tambien debe usar el
// M1 fresco de RecentCache, no solo M1 directo. El bucket M5 se arma
// plegando el M1 cacheado, sin necesidad de un cache propio por timeframe.
func TestGetCandles_DerivedTimeframeUpToNow_FoldsFreshM1FromCache(t *testing.T) {
	bucketStart := time.Date(2026, 8, 27, 18, 10, 0, 0, time.UTC)
	staleFromDB := domain.Candle{
		Symbol: "EMAT", Timeframe: domain.M5, Timestamp: bucketStart,
		Open: 3.15, High: 3.15, Low: 3.15, Close: 3.15, Volume: 100, // "vista antes de completarse"
	}
	repo := &fakeRepo{getResult: []domain.Candle{staleFromDB}}

	cache := livecandles.NewDefaultRecentCache()
	// Las velas M1 reales del bucket 18:10-18:15, con el volumen real
	// (27165 en el minuto del pico, como el caso real).
	m1s := []domain.Candle{
		{Symbol: "EMAT", Timeframe: domain.M1, Timestamp: bucketStart, Open: 3.18, High: 3.18, Low: 3.18, Close: 3.18, Volume: 200},
		{Symbol: "EMAT", Timeframe: domain.M1, Timestamp: bucketStart.Add(time.Minute), Open: 3.18, High: 3.18, Low: 3.18, Close: 3.18, Volume: 400},
		{Symbol: "EMAT", Timeframe: domain.M1, Timestamp: bucketStart.Add(3 * time.Minute), Open: 3.18, High: 3.18, Low: 3.18, Close: 3.18, Volume: 7000},
		{Symbol: "EMAT", Timeframe: domain.M1, Timestamp: bucketStart.Add(4 * time.Minute), Open: 3.15, High: 3.15, Low: 3.06, Close: 3.06, Volume: 27165},
	}
	for _, c := range m1s {
		cache.Put(c, true)
	}

	svc := ingestion.NewGetCandlesService(repo, cache)

	got, err := svc.GetCandles(context.Background(), "EMAT", domain.M5, 10, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 folded M5 candle, got %d", len(got))
	}
	const wantVolume = 200 + 400 + 7000 + 27165
	if got[0].Volume != wantVolume {
		t.Errorf("expected the folded volume %d (from fresh M1), got %d (stale DB value was %d)", wantVolume, got[0].Volume, staleFromDB.Volume)
	}
	if got[0].Close != 3.06 {
		t.Errorf("expected close 3.06 (from fresh M1), got %v", got[0].Close)
	}
}

// Regression: un timeframe cuyo bucket es mas ancho que lo que RecentCache
// alcanza a cubrir (H1 = 60 M1, el cache retiene ~20) NO debe tocarse --
// armar ese bucket con las pocas M1 que hay produciria un OHLCV incompleto
// (le falta la parte de atras, que ya salio de la ventana del cache) Y
// ademas duplicaria la fila que ya traia la BD para ese mismo bucket.
// Regression 2026-09-12: 2 pedidos concurrentes de la MISMA clave (simbolo +
// timeframe + bars) en un cache-miss pagaban 2 consultas reales a Postgres
// -- confirmado en vivo el mismo dia con pivots del frontend haciendo fila
// detras de escaneres. GetCandles debe colapsarlos en una sola llamada al
// repo (singleflight), sin importar cuantos pedidos identicos lleguen a la
// vez.
func TestGetCandles_ConcurrentIdenticalRequests_CollapseIntoOneFetch(t *testing.T) {
	repo := &fakeRepo{
		getResult:         []domain.Candle{{Symbol: "AAPL"}},
		getCandlesStarted: make(chan struct{}, 1),
		getCandlesGate:    make(chan struct{}),
	}
	svc := ingestion.NewGetCandlesService(repo, livecandles.NewDefaultRecentCache())

	type result struct {
		candles []domain.Candle
		err     error
	}
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func() {
			got, err := svc.GetCandles(context.Background(), "AAPL", domain.D1, 10, nil)
			results <- result{got, err}
		}()
	}

	// Solo debe llegar UNA señal de arranque -- singleflight.Do bloquea al
	// segundo caller ANTES de que toque el repo, nunca llega a ejecutarlo.
	<-repo.getCandlesStarted
	close(repo.getCandlesGate)

	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			t.Fatalf("unexpected error: %v", r.err)
		}
		if len(r.candles) != 1 {
			t.Fatalf("got %d candles, want 1", len(r.candles))
		}
	}
	if repo.getCandlesCalls != 1 {
		t.Errorf("expected exactly 1 real fetch for 2 concurrent identical requests, got %d", repo.getCandlesCalls)
	}
}

// Regression 2026-09-12: 2 llamadas concurrentes a GetCandlesBatch de
// escaneres DISTINTOS con universos solapados no debian pagar la misma
// consulta para el simbolo compartido -- confirmado en vivo como una fuente
// real de trabajo duplicado (varios escaneres corriendo ~cada 5min con alta
// superposicion entre si). El caller B pide SOLO el simbolo compartido (sin
// nada propio): si el coalescer funciona, B nunca debe tocar el repo, y
// el unico llamado real (del caller A) debe pedir los 2 simbolos de A.
func TestGetCandlesBatch_ConcurrentOverlappingCallers_ShareOneFetchForCommonSymbol(t *testing.T) {
	repo := &fakeRepo{
		seriesResult: map[string][]domain.Candle{
			"AAPL": {{Symbol: "AAPL", Timeframe: domain.D1}},
			"MSFT": {{Symbol: "MSFT", Timeframe: domain.D1}},
		},
		getSeriesStarted: make(chan struct{}, 1),
		getSeriesGate:    make(chan struct{}),
	}
	svc := ingestion.NewGetCandlesService(repo, livecandles.NewDefaultRecentCache())

	type result struct{ got map[string][]domain.Candle }
	resultsA := make(chan result, 1)
	resultsB := make(chan result, 1)

	go func() {
		got := svc.GetCandlesBatch(context.Background(), []string{"AAPL", "MSFT"}, domain.D1, 10)
		resultsA <- result{got}
	}()
	// Esperar a que A ya haya reclamado (claim) y este bloqueada DENTRO del
	// fetch real, antes de arrancar B -- asi B encuentra "MSFT" ya en vuelo
	// por A de forma deterministica, no por suerte de scheduling.
	<-repo.getSeriesStarted

	go func() {
		got := svc.GetCandlesBatch(context.Background(), []string{"MSFT"}, domain.D1, 10)
		resultsB <- result{got}
	}()
	// B no debe tocar el repo en absoluto (su unico simbolo ya esta en
	// vuelo) -- si lo hiciera, quedaria bloqueada esperando el gate y este
	// segundo receive de getSeriesStarted nunca llegaria a tiempo. Un pequeño
	// margen es suficiente porque claim() no bloquea (es solo un mutex).
	select {
	case <-repo.getSeriesStarted:
		t.Fatal("caller B no debia llamar a GetSeries -- su unico simbolo ya estaba en vuelo por A")
	case <-time.After(50 * time.Millisecond):
	}

	close(repo.getSeriesGate)

	rA := <-resultsA
	rB := <-resultsB

	if len(rA.got["AAPL"]) != 1 || len(rA.got["MSFT"]) != 1 {
		t.Fatalf("caller A: resultado incompleto: %+v", rA.got)
	}
	if len(rB.got["MSFT"]) != 1 {
		t.Fatalf("caller B: no recibio MSFT via coalescer: %+v", rB.got)
	}
	if len(repo.getSeriesCalls) != 1 {
		t.Fatalf("esperaba exactamente 1 llamada real a GetSeries, hubo %d: %v", len(repo.getSeriesCalls), repo.getSeriesCalls)
	}
}

// Regression 2026-09-11: un lote de un timeframe BASE (D1, tambien M1) caia
// siempre a getCandlesBatchPerSymbol (4 workers, 2 round trips por simbolo) --
// con RANGE_EXTREME_PROXIMITY evaluando D1 sobre miles de simbolos, eso
// disparaba timeouts reales en produccion. GetSeries ya resuelve el lote
// completo en una sola consulta (usado por RefreshBeta); GetCandlesBatch debe
// usarlo para D1/M1 igual que ya usa GetSeriesAggregatedBatch para H1/M15/etc.
func TestGetCandlesBatch_BaseTimeframe_UsesSeriesNotPerSymbolFallback(t *testing.T) {
	repo := &fakeRepo{
		seriesResult: map[string][]domain.Candle{
			"AAPL": {{Symbol: "AAPL", Timeframe: domain.D1, Close: 230}},
			"MSFT": {{Symbol: "MSFT", Timeframe: domain.D1, Close: 410}},
		},
	}
	svc := ingestion.NewGetCandlesService(repo, livecandles.NewDefaultRecentCache())

	got := svc.GetCandlesBatch(context.Background(), []string{"AAPL", "MSFT"}, domain.D1, 10)

	if len(got) != 2 {
		t.Fatalf("got %d symbols, want 2", len(got))
	}
	if got["AAPL"][0].Close != 230 || got["MSFT"][0].Close != 410 {
		t.Errorf("unexpected candle data: %+v", got)
	}
	if repo.getCandlesCalls != 0 {
		t.Errorf("expected the slow per-symbol fallback to NOT run, but GetCandles was called %d times", repo.getCandlesCalls)
	}
}

// Si GetSeries falla, el lote debe seguir resolviendose (degradado) por el
// camino per-simbolo de siempre, no devolver vacio.
func TestGetCandlesBatch_BaseTimeframe_FallsBackToPerSymbolOnSeriesError(t *testing.T) {
	repo := &fakeRepo{
		seriesErr: errors.New("boom"),
		getResult: []domain.Candle{{Symbol: "AAPL", Timeframe: domain.D1, Close: 230}},
	}
	svc := ingestion.NewGetCandlesService(repo, livecandles.NewDefaultRecentCache())

	got := svc.GetCandlesBatch(context.Background(), []string{"AAPL"}, domain.D1, 10)

	if len(got) != 1 || got["AAPL"][0].Close != 230 {
		t.Fatalf("expected the per-symbol fallback result, got %+v", got)
	}
	if repo.getCandlesCalls == 0 {
		t.Errorf("expected the per-symbol fallback to run when GetSeries fails")
	}
}

func TestGetCandles_BucketWiderThanCacheCoverage_LeavesBaseUntouched(t *testing.T) {
	hourStart := time.Date(2026, 8, 27, 18, 0, 0, 0, time.UTC)
	fromDB := domain.Candle{
		Symbol: "EMAT", Timeframe: domain.H1, Timestamp: hourStart,
		Open: 3.20, High: 3.20, Low: 3.00, Close: 3.10, Volume: 500000,
	}
	repo := &fakeRepo{getResult: []domain.Candle{fromDB}}

	cache := livecandles.NewDefaultRecentCache()
	// Solo unos pocos M1 recientes (bien lejos de cubrir la hora entera) --
	// simula el cache real, que solo retiene ~20 barras.
	cache.Put(domain.Candle{Symbol: "EMAT", Timeframe: domain.M1, Timestamp: hourStart.Add(40 * time.Minute), Close: 3.06, Volume: 27165}, true)
	cache.Put(domain.Candle{Symbol: "EMAT", Timeframe: domain.M1, Timestamp: hourStart.Add(41 * time.Minute), Close: 3.08, Volume: 11453}, true)

	svc := ingestion.NewGetCandlesService(repo, cache)

	got, err := svc.GetCandles(context.Background(), "EMAT", domain.H1, 10, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected the original DB bucket untouched (1 candle), got %d", len(got))
	}
	if got[0].Volume != fromDB.Volume || got[0].Close != fromDB.Close {
		t.Errorf("expected the DB candle unchanged (volume=%d close=%v), got volume=%d close=%v",
			fromDB.Volume, fromDB.Close, got[0].Volume, got[0].Close)
	}
}
