package catchup

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
)

type batchOutcome struct {
	saved       int
	empty       []string
	cut         int
	upToDate    int
	skippedLive int
	errored     int
	noWatermark int
	partial     bool
	accounted   []string
}

func (o *batchOutcome) addAccounted(syms ...string) {
	o.accounted = append(o.accounted, syms...)
}

func (o *batchOutcome) recordBackfill(ctx context.Context, ingest in.IngestCandlesService, j job) {
	switch backfillOutcome(ctx, ingest, j) {
	case outcomeSaved:
		o.saved++
	case outcomeEmpty:
		o.empty = append(o.empty, j.symbol)
	default:
		o.errored++
	}
	o.addAccounted(j.symbol)
}
