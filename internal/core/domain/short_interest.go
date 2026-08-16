package domain

// ShortInterestRecord es una fila del CSV biweekly de FINRA. SettlementDate
// permite detectar si el archivo mas reciente ya se proceso -- no cambia
// entre visitas del mismo periodo, no vale la pena reprocesarlo.
type ShortInterestRecord struct {
	SharesShorted  int64
	AvgDailyVolume int64
	DaysToCover    float64
	SettlementDate string
}
