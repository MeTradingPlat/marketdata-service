package domain

import "time"

// betaMinAlignedMonths: con menos meses solapados entre el simbolo y el
// proxy de mercado el beta es ruido estadistico, no una medida -- se
// prefiere no dar dato (y que el llamador conserve el beta de TastyTrade)
// antes que un numero que depende de 6 observaciones.
const betaMinAlignedMonths = 24

// BetaFallbackMonths es el minimo relajado (beta 1Y, convencion valida en
// la industria para simbolos con historia corta) que el refresh usa solo
// cuando ni la historia propia llega a 2 anios ni TastyTrade trae beta --
// un beta 1Y real es mejor que N/A (confirmado en vivo: MDXH, ADR con ~19
// meses de D1, quedaba sin beta del todo).
const BetaFallbackMonths = 12

// BetaFallbackWeeks: retornos semanales de 1 anio (52 observaciones, vs 12
// mensuales) -- como hacen Bloomberg/Reuters para ventanas cortas: mas
// observaciones que mensuales, mas robusto. Cubre los simbolos con 12-23
// meses de historia donde el beta 5Y mensual no aplica.
const BetaFallbackWeeks = 52

// BetaMinWeeks es el ultimo escalon real: 6 meses de retornos semanales
// (26 observaciones) para simbolos con 6-11 meses de D1 -- un beta
// semestral semanal es mejor que N/A, y es lo mas cerca de un dato real
// que se puede llegar con historia tan corta (menos que esto es ruido).
const BetaMinWeeks = 26

// MonthlyBeta calcula el beta clasico (cov(simbolo, mercado)/var(mercado))
// sobre retornos MENSUALES de cierres D1 -- la misma convencion de la
// industria (5Y monthly, ver fuentes de stockanalysis/Yahoo), no la
// ventana corta que usa TastyTrade y que da betas raros en ADRs/bajo
// volumen (ABEV 0.70 vs 0.26 real, NOK 1.43 vs 0.76 real).
//
// symbolCandles y marketCandles son velas D1 en orden ascendente. Si no
// hay al menos betaMinAlignedMonths meses solapados, devuelve nil.
func MonthlyBeta(symbolCandles, marketCandles []Candle) *float64 {
	return monthlyBetaMin(symbolCandles, marketCandles, betaMinAlignedMonths)
}

// MonthlyBetaMin es la misma covarianza mensual con un minimo de meses
// explicito (ver betaFallbackMonths para el caso de uso).
func MonthlyBetaMin(symbolCandles, marketCandles []Candle, minMonths int) *float64 {
	return monthlyBetaMin(symbolCandles, marketCandles, minMonths)
}

func monthlyBetaMin(symbolCandles, marketCandles []Candle, minMonths int) *float64 {
	return covarianceBeta(monthlyCloses(symbolCandles), monthlyCloses(marketCandles), prevMonth, minMonths)
}

// WeeklyBetaMin es la misma covarianza sobre retornos SEMANALES (ISO week,
// alineados simbolo<->mercado) -- el fallback de historia corta: 52
// observaciones por anio en vez de 12 mensuales, la convencion de
// Bloomberg/Reuters para ventanas de 1-2 anios (ver BetaFallbackWeeks y
// BetaMinWeeks).
func WeeklyBetaMin(symbolCandles, marketCandles []Candle, minWeeks int) *float64 {
	return covarianceBeta(weeklyCloses(symbolCandles), weeklyCloses(marketCandles), prevWeek, minWeeks)
}

// covarianceBeta es el nucleo clasico cov(simbolo, mercado)/var(mercado)
// sobre retornos por periodo (mensual o semanal segun el mapa de cierres y
// la funcion de periodo anterior) -- requiere al menos minObs periodos
// solapados con su periodo anterior completo.
func covarianceBeta(symbol, market map[int]float64, prev func(int) int, minObs int) *float64 {
	if len(symbol) < minObs || len(market) < minObs {
		return nil
	}

	var xs, ys []float64
	for period, mClose := range market {
		sClose, ok := symbol[period]
		if !ok || sClose <= 0 || mClose <= 0 {
			continue
		}
		prevM, prevMok := market[prev(period)]
		prevS, prevSok := symbol[prev(period)]
		if !prevMok || !prevSok || prevM <= 0 || prevS <= 0 {
			continue
		}
		xs = append(xs, mClose/prevM-1)
		ys = append(ys, sClose/prevS-1)
	}
	if len(xs) < minObs {
		return nil
	}

	meanX := mean(xs)
	meanY := mean(ys)
	var cov, varX float64
	for i := range xs {
		dx := xs[i] - meanX
		cov += dx * (ys[i] - meanY)
		varX += dx * dx
	}
	if varX == 0 {
		return nil
	}
	beta := cov / varX
	return &beta
}

func monthlyCloses(candles []Candle) map[int]float64 {
	closes := make(map[int]float64)
	for _, c := range candles {
		if c.Close <= 0 {
			continue
		}
		key := c.Timestamp.Year()*100 + int(c.Timestamp.Month())
		closes[key] = c.Close
	}
	return closes
}

func prevMonth(monthKey int) int {
	year := monthKey / 100
	month := monthKey % 100
	if month == 1 {
		return (year-1)*100 + 12
	}
	return monthKey - 1
}

func weeklyCloses(candles []Candle) map[int]float64 {
	closes := make(map[int]float64)
	for _, c := range candles {
		if c.Close <= 0 {
			continue
		}
		year, week := c.Timestamp.ISOWeek()
		closes[year*100+week] = c.Close
	}
	return closes
}

func prevWeek(weekKey int) int {
	year := weekKey / 100
	week := weekKey % 100
	if week > 1 {
		return weekKey - 1
	}
	// La semana 1 de un anio sigue a la ultima semana ISO del anio
	// anterior (52 o 53) -- el 28 de diciembre siempre cae en la ultima
	// semana ISO de su anio.
	prevYearEnd := time.Date(year-1, 12, 28, 0, 0, 0, 0, time.UTC)
	_, lastWeek := prevYearEnd.ISOWeek()
	return (year-1)*100 + lastWeek
}

func mean(vals []float64) float64 {
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}
