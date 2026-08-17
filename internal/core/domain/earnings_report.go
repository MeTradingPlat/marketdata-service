package domain

// EarningsReportItem es un reporte de earnings pasado con EPS ya real
// (no estimado) -- viene de un endpoint separado de TastyTrade
// (historic-corporate-events/earnings-reports), no de /market-metrics.
// Solo se conserva OccurredDate: el EPS ya lo trae /market-metrics con
// su propio refresh nocturno, no hace falta duplicarlo aca.
type EarningsReportItem struct {
	OccurredDate string
}
