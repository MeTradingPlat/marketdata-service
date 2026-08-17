package catchup

import (
	"math"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// heuristicFloatRatio: apenas se conoce sharesOutstanding (sea nuevo o
// actualizado), floatShares se estima al 90% hasta que
// RefreshBeneficialOwners lo reemplace por el real -- mejor una estimacion
// razonable de inmediato que dejar floatShares en null indefinidamente
// mientras el loop de holders 5%+ (por-simbolo, lento) le llega el turno.
const heuristicFloatRatio = 0.90

// maxFinraSettlementAge: FINRA publica quincenal (~14 dias + ~1 semana de
// lag de publicacion). Un settlement con mas de esto de antiguedad
// significa que la publicacion se atraso o algo fallo -- se descarta el
// dato en vez de mostrar un numero viejo como si fuera vigente.
const maxFinraSettlementAge = 35 * 24 * time.Hour

func isFinraStale(settlementDate string) bool {
	if settlementDate == "" {
		return true
	}
	t, err := time.Parse("2006-01-02", settlementDate)
	if err != nil {
		return true
	}
	return time.Since(t) > maxFinraSettlementAge
}

func buildExternalFundamentals(symbols []string, sharesOutstanding map[string]int64, insiderData map[string]domain.InsiderOwnership, finraData map[string]domain.ShortInterestRecord) []domain.Fundamentals {
	updates := make([]domain.Fundamentals, 0, len(symbols))
	for _, symbol := range symbols {
		f := domain.Fundamentals{Symbol: symbol}
		hasUpdate := false

		if shares, ok := sharesOutstanding[symbol]; ok {
			f.SharesOutstanding = &shares
			heuristic := int64(math.Round(float64(shares) * heuristicFloatRatio))
			f.FloatShares = &heuristic
			hasUpdate = true
		}

		if ownership, ok := insiderData[symbol]; ok {
			shares := ownership.Shares
			f.InsiderShares = &shares
			f.InsiderCiks = ciksToSlice(ownership.OwnerCiks)
			hasUpdate = true
		}

		if rec, ok := finraData[symbol]; ok {
			if !isFinraStale(rec.SettlementDate) {
				if rec.DaysToCover > 0 && rec.DaysToCover < 999 {
					shortRatio := rec.DaysToCover
					f.ShortRatio = &shortRatio
				}
				if rec.SharesShorted > 0 {
					sharesShorted := rec.SharesShorted
					f.ShortInterestShares = &sharesShorted
				}
				f.ShortInterestSettlement = rec.SettlementDate
			}
			hasUpdate = true
		}

		if hasUpdate {
			updates = append(updates, f)
		}
	}
	return updates
}

func ciksToSlice(ciks map[string]bool) []string {
	result := make([]string, 0, len(ciks))
	for cik := range ciks {
		result = append(result, cik)
	}
	return result
}
