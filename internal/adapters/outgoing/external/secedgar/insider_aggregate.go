package secedgar

import (
	"sort"
	"strings"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type latestHolding struct {
	symbol     string
	filingDate time.Time
	shares     int64
	owners     map[string]bool
}

// aggregateLatestPerOwnerGroup se queda con el filing mas reciente POR
// GRUPO DE OWNERS (no por accession individual) -- varios filings del mismo
// grupo de reporting owners a lo largo de los 8 trimestres son la misma
// posicion actualizandose, sumarlos todos multiplicaria la tenencia real.
func aggregateLatestPerOwnerGroup(accessions map[string]accessionInfo, ownerGroups map[string]map[string]bool, holdingsByAccession map[string]int64) map[string]domain.InsiderOwnership {
	latestByGroup := make(map[string]latestHolding)
	for accession, info := range accessions {
		owners := ownerGroups[accession]
		if len(owners) == 0 {
			continue
		}
		groupKey := info.symbol + "|" + sortedJoin(owners)
		shares := holdingsByAccession[accession]
		current, exists := latestByGroup[groupKey]
		if !exists || info.filingDate.After(current.filingDate) {
			latestByGroup[groupKey] = latestHolding{symbol: info.symbol, filingDate: info.filingDate, shares: shares, owners: owners}
		}
	}

	sharesBySymbol := make(map[string]int64)
	ciksBySymbol := make(map[string]map[string]bool)
	for _, h := range latestByGroup {
		sharesBySymbol[h.symbol] += h.shares
		if ciksBySymbol[h.symbol] == nil {
			ciksBySymbol[h.symbol] = make(map[string]bool)
		}
		for cik := range h.owners {
			ciksBySymbol[h.symbol][cik] = true
		}
	}

	result := make(map[string]domain.InsiderOwnership, len(sharesBySymbol))
	for symbol, shares := range sharesBySymbol {
		result[symbol] = domain.InsiderOwnership{Shares: shares, OwnerCiks: ciksBySymbol[symbol]}
	}
	return result
}

func sortedJoin(set map[string]bool) string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}
