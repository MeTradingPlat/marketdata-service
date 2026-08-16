package finra

import (
	"strconv"
	"strings"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
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
			SharesShorted:  parseInt(cols[5]),
			AvgDailyVolume: parseInt(cols[8]),
			DaysToCover:    parseFloat(cols[9]),
		}
		if len(cols) > 13 {
			rec.SettlementDate = strings.TrimSpace(cols[13])
		}
		result[symbol] = rec
	}
	return result
}

func parseInt(v string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func parseFloat(v string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0
	}
	return f
}
