package domain

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
	symbol := monthlyCloses(symbolCandles)
	market := monthlyCloses(marketCandles)
	if len(symbol) < minMonths || len(market) < minMonths {
		return nil
	}

	var xs, ys []float64
	for month, mClose := range market {
		sClose, ok := symbol[month]
		if !ok || sClose <= 0 || mClose <= 0 {
			continue
		}
		prevM, prevMok := market[prevMonth(month)]
		prevS, prevSok := symbol[prevMonth(month)]
		if !prevMok || !prevSok || prevM <= 0 || prevS <= 0 {
			continue
		}
		xs = append(xs, mClose/prevM-1)
		ys = append(ys, sClose/prevS-1)
	}
	if len(xs) < minMonths {
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

func mean(vals []float64) float64 {
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}
