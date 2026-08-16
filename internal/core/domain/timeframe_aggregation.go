package domain

import "time"

// aggregationSpec dice, para un timeframe derivado, de cual timeframe base
// agruparlo y con que ancho de bucket. Confirmado contra dxFeed real: el
// alineamiento por defecto (CandleAlignment.MIDNIGHT, sin el parametro
// a=s de sesion) es UTC, no hora de mercado -- coincide con lo que ya
// vemos en M1/H1/D1 propios (D1 en T00:00:00Z, H1 en horas exactas UTC),
// asi que agrupar con limites UTC reproduce la misma alineacion que
// tendria un M5/H4/etc. nativo de TastyTrade, no una convencion inventada.
type aggregationSpec struct {
	Source Timeframe
	Bucket string
	// approxPeriod acota el rango de fecha de la consulta (igual que
	// GetCandles con timeframes base) -- no necesita ser exacto para
	// mes/año, solo lo bastante grande para no dejar afuera barras reales.
	approxPeriod time.Duration
}

var derivedTimeframes = map[Timeframe]aggregationSpec{
	M2:  {M1, "2 minutes", 2 * time.Minute},
	M3:  {M1, "3 minutes", 3 * time.Minute},
	M5:  {M1, "5 minutes", 5 * time.Minute},
	M10: {M1, "10 minutes", 10 * time.Minute},
	M15: {M1, "15 minutes", 15 * time.Minute},
	M30: {M1, "30 minutes", 30 * time.Minute},
	M45: {M1, "45 minutes", 45 * time.Minute},
	H2:  {H1, "2 hours", 2 * time.Hour},
	H3:  {H1, "3 hours", 3 * time.Hour},
	H4:  {H1, "4 hours", 4 * time.Hour},
	H12: {H1, "12 hours", 12 * time.Hour},
	D2:  {D1, "2 days", 2 * 24 * time.Hour},
	D3:  {D1, "3 days", 3 * 24 * time.Hour},
	W1:  {D1, "7 days", 7 * 24 * time.Hour},
	MO1: {D1, "1 month", 31 * 24 * time.Hour},
	MO3: {D1, "3 months", 93 * 24 * time.Hour},
	MO6: {D1, "6 months", 186 * 24 * time.Hour},
	Y1:  {D1, "1 year", 366 * 24 * time.Hour},
}

// Aggregation devuelve, para un timeframe derivado, el timeframe base del
// que agruparlo, el ancho de bucket (formato interval de Postgres) y un
// margen de fecha aproximado para acotar la consulta. ok es false para
// timeframes base (M1/H1/D1) o invalidos.
func (t Timeframe) Aggregation() (source Timeframe, bucket string, approxPeriod time.Duration, ok bool) {
	spec, ok := derivedTimeframes[t]
	return spec.Source, spec.Bucket, spec.approxPeriod, ok
}
