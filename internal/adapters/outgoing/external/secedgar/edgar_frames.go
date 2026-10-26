package secedgar

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

// Frames fast-path: 13317 simbolos medidos el 2026-10-25, 6 requests a
// /api/xbrl/frames (3 tags x 2 trimestres) tardaron ~1.2s en total y
// resolvieron ~59% de los CIKs que tenian CIK conocido. El resto cae al ZIP
// bulk como siempre -- Frames es solo un pre-paso, nunca un reemplazo.
//
// framesAttempts ordena los tags del mas al menos usado en esa medicion; ya
// fue validado contra el universo real, no reordenar sin volver a medir (el
// orden importa: matchFrameEntities no pisa un CIK ya resuelto, asi que el
// primer tag que lo resuelve gana).

type frameEntity struct {
	CIK        int64  `json:"cik"`
	EntityName string `json:"entityName"`
	Val        int64  `json:"val"`
	End        string `json:"end"`
}

type framesResponse struct {
	Data []frameEntity `json:"data"`
}

var framesAttempts = []struct{ taxonomy, tag string }{
	{"us-gaap", "CommonStockSharesOutstanding"},
	{"us-gaap", "SharesOutstanding"},
	{"dei", "EntityCommonStockSharesOutstanding"},
}

// currentAndPreviousQuarterInstant calcula los periodos "instant" de XBRL
// (CY{year}Q{quarter}I) sin hardcodear año ni trimestre: esta codigo sigue
// vivo en trimestres futuros. Al cruzar de año, el trimestre anterior es Q4
// del año previo.
func currentAndPreviousQuarterInstant(now time.Time) (current, previous string) {
	q := (int(now.Month())-1)/3 + 1
	year := now.Year()
	current = fmt.Sprintf("CY%dQ%dI", year, q)
	q--
	if q == 0 {
		q = 4
		year--
	}
	previous = fmt.Sprintf("CY%dQ%dI", year, q)
	return current, previous
}

// matchFrameEntities aplica la misma regla de plausibilidad/antiguedad que el
// path del ZIP (minPlausibleShares, isStale) y asigna la cifra del CIK a
// todos sus tickers (el emisor y sus warrants/preferentes comparten el dato).
func matchFrameEntities(entities []frameEntity, cikToSymbols map[string][]string, resolvedCiks map[string]bool, shares map[string]int64) {
	for _, pt := range entities {
		cik := fmt.Sprintf("%010d", pt.CIK)
		if resolvedCiks[cik] {
			continue
		}
		if _, wanted := cikToSymbols[cik]; !wanted {
			continue
		}
		if pt.Val < minPlausibleShares || isStale(pt.End) {
			continue
		}
		resolvedCiks[cik] = true
		for _, symbol := range cikToSymbols[cik] {
			shares[symbol] = pt.Val
		}
	}
}

func fetchFramesShares(ctx context.Context, cikToSymbols map[string][]string) (shares map[string]int64, resolvedCiks map[string]bool) {
	shares = make(map[string]int64)
	resolvedCiks = make(map[string]bool)

	current, previous := currentAndPreviousQuarterInstant(time.Now())
	periods := []string{current, previous}

	for _, period := range periods {
		for _, attempt := range framesAttempts {
			url := fmt.Sprintf("https://data.sec.gov/api/xbrl/frames/%s/%s/shares/%s.json", attempt.taxonomy, attempt.tag, period)
			body, err := fetchBody(ctx, url)
			if err != nil {
				log.Warn().Err(err).Str("taxonomy", attempt.taxonomy).Str("tag", attempt.tag).Str("period", period).Msg("sec edgar frames request failed, skipping")
				continue
			}
			var resp framesResponse
			if err := json.Unmarshal(body, &resp); err != nil {
				log.Warn().Err(err).Str("taxonomy", attempt.taxonomy).Str("tag", attempt.tag).Str("period", period).Msg("sec edgar frames response unparseable, skipping")
				continue
			}
			matchFrameEntities(resp.Data, cikToSymbols, resolvedCiks, shares)
		}
	}
	return shares, resolvedCiks
}
