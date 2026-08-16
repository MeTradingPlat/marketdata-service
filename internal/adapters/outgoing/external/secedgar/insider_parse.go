package secedgar

import (
	"strings"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type accessionInfo struct {
	symbol     string
	filingDate time.Time
}

func parseInsiderHoldings(zipFiles []string, targets map[string]bool) map[string]domain.InsiderOwnership {
	accessions := make(map[string]accessionInfo)
	for _, zf := range zipFiles {
		forEachRow(zf, "SUBMISSION.tsv", func(headers, cols []string) {
			symbol := strings.ToUpper(value(headers, cols, "ISSUERTRADINGSYMBOL"))
			if symbol == "" || !targets[symbol] {
				return
			}
			accession := value(headers, cols, "ACCESSION_NUMBER")
			accessions[accession] = accessionInfo{symbol: symbol, filingDate: parseDate(value(headers, cols, "FILING_DATE"))}
		})
	}
	if len(accessions) == 0 {
		return map[string]domain.InsiderOwnership{}
	}

	ownerGroups := make(map[string]map[string]bool)
	holdingsByAccession := make(map[string]int64)
	for _, zf := range zipFiles {
		forEachRow(zf, "REPORTINGOWNER.tsv", func(headers, cols []string) {
			accession := value(headers, cols, "ACCESSION_NUMBER")
			if _, ok := accessions[accession]; !ok {
				return
			}
			if ownerGroups[accession] == nil {
				ownerGroups[accession] = make(map[string]bool)
			}
			ownerGroups[accession][value(headers, cols, "RPTOWNERCIK")] = true
		})
		forEachRow(zf, "NONDERIV_HOLDING.tsv", func(headers, cols []string) {
			accession := value(headers, cols, "ACCESSION_NUMBER")
			if _, ok := accessions[accession]; !ok {
				return
			}
			holdingsByAccession[accession] += parseSharesInt(value(headers, cols, "SHRS_OWND_FOLWNG_TRANS"))
		})
	}

	return aggregateLatestPerOwnerGroup(accessions, ownerGroups, holdingsByAccession)
}
