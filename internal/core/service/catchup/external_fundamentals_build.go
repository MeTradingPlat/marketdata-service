package catchup

import (
	"math"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// heuristicFloatRatio: apenas se conoce sharesOutstanding (sea nuevo o
// actualizado), floatShares se estima al 90% hasta que
// RefreshBeneficialOwners lo reemplace por el real -- mejor una estimacion
// razonable de inmediato que dejar floatShares en null indefinidamente
// mientras el loop de holders 5%+ (por-simbolo, lento) le llega el turno.
const heuristicFloatRatio = 0.90

// maxPlausibleShortInterestPct: por encima de esto, sharesShorted/floatShares
// casi siempre significa que floatShares esta mal (ej. heuristico sobre un
// sharesOutstanding stale) y no que el 300%+ del float este realmente en
// corto -- se prefiere omitir el dato a mostrar un numero absurdo.
const maxPlausibleShortInterestPct = 300.0

func buildExternalFundamentals(symbols []string, sharesOutstanding map[string]int64, insiderData map[string]domain.InsiderOwnership, existing map[string]domain.Fundamentals, finraData map[string]domain.ShortInterestRecord) []domain.Fundamentals {
	updates := make([]domain.Fundamentals, 0, len(symbols))
	for _, symbol := range symbols {
		f := domain.Fundamentals{Symbol: symbol}
		hasUpdate := false

		floatShares := existingFloat(existing, symbol)
		if shares, ok := sharesOutstanding[symbol]; ok {
			f.SharesOutstanding = &shares
			heuristic := int64(math.Round(float64(shares) * heuristicFloatRatio))
			f.FloatShares = &heuristic
			floatShares = &heuristic
			hasUpdate = true
		}

		if ownership, ok := insiderData[symbol]; ok {
			shares := ownership.Shares
			f.InsiderShares = &shares
			f.InsiderCiks = ciksToSlice(ownership.OwnerCiks)
			hasUpdate = true
		}

		if rec, ok := finraData[symbol]; ok {
			if rec.DaysToCover > 0 && rec.DaysToCover < 999 {
				shortRatio := rec.DaysToCover
				f.ShortRatio = &shortRatio
			}
			if shortInterest := computeShortInterestPercent(rec.SharesShorted, floatShares); shortInterest != nil {
				f.ShortInterest = shortInterest
			}
			hasUpdate = true
		}

		if hasUpdate {
			updates = append(updates, f)
		}
	}
	return updates
}

func existingFloat(existing map[string]domain.Fundamentals, symbol string) *int64 {
	if f, ok := existing[symbol]; ok {
		return f.FloatShares
	}
	return nil
}

func ciksToSlice(ciks map[string]bool) []string {
	result := make([]string, 0, len(ciks))
	for cik := range ciks {
		result = append(result, cik)
	}
	return result
}

func computeShortInterestPercent(sharesShorted int64, floatShares *int64) *float64 {
	if floatShares == nil || *floatShares <= 0 || sharesShorted <= 0 {
		return nil
	}
	pct := math.Round(float64(sharesShorted)/float64(*floatShares)*100*100) / 100
	if pct > maxPlausibleShortInterestPct {
		return nil
	}
	return &pct
}
