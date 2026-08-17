package domain

import "time"

// M1DayHole es un dia con actividad (>= m1HoleMinBars velas) donde el
// conteo de velas es menor que el lapso en minutos entre la primera y la
// ultima -- hay minutos faltantes EN MEDIO del dia (ej. hueco de un
// deploy), no solo minutos muertos de un simbolo sin volumen (esos no se
// marcan). Lo detecta CandleRepository.GetM1DayHoles y lo rellena
// FillM1Gaps.
type M1DayHole struct {
	Symbol string
	Day    time.Time
}
