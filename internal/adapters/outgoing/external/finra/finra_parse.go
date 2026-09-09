package finra

import (
	"strings"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/pkg/numparse"
)

func parseFinraCsv(body []byte) map[string]domain.ShortInterestRecord {
	lines := strings.Split(string(body), "\n")
	if len(lines) == 0 {
		return nil
	}
	header := strings.TrimSpace(lines[0])
	if header == "" || strings.HasPrefix(header, "<?xml") {
		return nil
	}

	result := make(map[string]domain.ShortInterestRecord)
	for _, line := range lines[1:] {
		cols := strings.Split(line, "|")
		if len(cols) < 10 {
			continue
		}
		symbol := strings.ToUpper(strings.TrimSpace(cols[1]))
		if symbol == "" {
			continue
		}
		rec := domain.ShortInterestRecord{
			SharesShorted:  numparse.Int(cols[5]),
			AvgDailyVolume: numparse.Int(cols[8]),
			DaysToCover:    numparse.Float(cols[9]),
		}
		if len(cols) > 13 {
			rec.SettlementDate = strings.TrimSpace(cols[13])
		}
		result[symbol] = rec
	}
	return result
}
