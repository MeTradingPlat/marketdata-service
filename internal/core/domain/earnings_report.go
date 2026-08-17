package domain

// EarningsReportItem es un reporte de earnings pasado con EPS ya real
// (no estimado) -- viene de un endpoint separado de TastyTrade
// (historic-corporate-events/earnings-reports), no de /market-metrics.
type EarningsReportItem struct {
	OccurredDate string
	Eps          float64
}
