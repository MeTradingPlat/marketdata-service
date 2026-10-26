package secedgar

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
)

const bulkFactsURL = "https://www.sec.gov/Archives/edgar/daily-index/xbrl/companyfacts.zip"

// EdgarClient resuelve sharesOutstanding contra el bulk companyfacts.zip de
// la SEC (~1.5GB), cacheado en disco una vez por dia y leido entrada por
// entrada -- la memoria pico queda acotada al JSON de una sola empresa, no
// al archivo completo (ver parseCompanyFactsZip).
type EdgarClient struct {
	tickerCik *TickerCikLookup
	cacheDir  string
}

func NewEdgarClient(tickerCik *TickerCikLookup, cacheDir string) *EdgarClient {
	return &EdgarClient{tickerCik: tickerCik, cacheDir: cacheDir}
}

func (c *EdgarClient) FetchSharesOutstanding(ctx context.Context, symbols []string) map[string]int64 {
	shares, _ := c.FetchCompanyFacts(ctx, symbols)
	return shares
}

// FetchCompanyFacts trae sharesOutstanding y la fecha del ultimo filing
// (10-Q/10-K) en UN solo pase sobre el bulk -- usar esto en vez de llamar
// FetchSharesOutstanding aparte cuando el caller necesita ambos datos, para
// no decodificar el ZIP completo (~1.5GB) dos veces en la misma corrida.
func (c *EdgarClient) FetchCompanyFacts(ctx context.Context, symbols []string) (shares map[string]int64, filingDates map[string]string) {
	cikToSymbols := make(map[string][]string)
	tickerToCik := c.tickerCik.TickerToCikMap(ctx)
	for _, symbol := range symbols {
		cik, ok := tickerToCik[strings.ToUpper(symbol)]
		if !ok {
			continue
		}
		key := fmt.Sprintf("%010d", cik)
		cikToSymbols[key] = append(cikToSymbols[key], strings.ToUpper(symbol))
	}
	if len(cikToSymbols) == 0 {
		return map[string]int64{}, map[string]string{}
	}

	// Pre-paso Frames: resuelve una parte del universo en ~1s; solo los CIKs
	// que Frames no resolvio bajan al ZIP bulk. filingDates sale SIEMPRE del
	// ZIP (Frames solo tiene fin de periodo, no fecha de filing real: meterlo
	// en filingDates corromperia la semantica de "ultimo filing").
	shares, resolvedCiks := fetchFramesShares(ctx, cikToSymbols)
	log.Info().Int("ciks", len(resolvedCiks)).Int("symbols", len(shares)).Msg("sec edgar frames shares resolved")

	remaining := make(map[string][]string, len(cikToSymbols)-len(resolvedCiks))
	for cik, syms := range cikToSymbols {
		if !resolvedCiks[cik] {
			remaining[cik] = syms
		}
	}
	if len(remaining) == 0 {
		return shares, map[string]string{}
	}

	// sharesOutstanding es una cifra por-entidad (CIK), no por-ticker: se
	// aplica a todos los tickers del CIK (el principal y sus warrants/
	// preferentes del mismo emisor). Descartar el CIK con mas de un ticker
	// dejaba al principal legitimo sin shares (T, SMCI, OPEN, BBD), peor que
	// asignar el SO del emisor a sus instrumentos.
	totalTargets := len(remaining)

	zipPath, err := c.ensureCachedZip(ctx)
	if err != nil {
		log.Error().Err(err).Str("cacheDir", c.cacheDir).Msg("sec edgar companyfacts.zip unavailable")
		return shares, map[string]string{}
	}
	zipShares, filingDates := parseCompanyFactsZip(zipPath, remaining, totalTargets)
	for symbol, val := range zipShares {
		shares[symbol] = val
	}
	return shares, filingDates
}
