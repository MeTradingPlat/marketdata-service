package intraday

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// mergeFormingCandle suma la vela M1 en formacion (todavia no guardada, la
// unica fuente que llega hasta el ultimo tick real) a la sesion que le
// corresponde ahora -- sin esto, high/low/volumen del dia quedan hasta un
// minuto atras del precio actual, que si la incluye.
func mergeFormingCandle(snap *domain.IntradaySnapshot, current domain.Candle) {
	mergeFormingCandleAt(snap, current, time.Now())
}

func mergeFormingCandleAt(snap *domain.IntradaySnapshot, current domain.Candle, now time.Time) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return
	}

	nowET := now.In(loc)
	currentET := current.Timestamp.In(loc)
	// p.current se vacia por completo en cada frontera del barrido nocturno,
	// pero justo despues de medianoche (ET) puede seguir viva la ultima vela
	// de AYER hasta que llegue el primer tick real de hoy -- sin este chequeo
	// esa vela se clasificaria igual (post-market de ayer) pero mezclada
	// dentro del snapshot de HOY, que la query SQL ya calculo vacio.
	if currentET.Year() != nowET.Year() || currentET.YearDay() != nowET.YearDay() {
		return
	}

	hours := domain.NYSESessionHoursFor(nowET)
	if current.Timestamp.Before(hours.PreOpen) || !current.Timestamp.Before(hours.PostEnd) {
		return
	}

	switch {
	case current.Timestamp.Before(hours.Open):
		snap.PreMarketVolume += current.Volume
		snap.PreMarketClose = current.Close
	case current.Timestamp.Before(hours.Close):
		if snap.Open == 0 {
			snap.Open = current.Open
		}
		if snap.High == 0 || current.High > snap.High {
			snap.High = current.High
		}
		if snap.Low == 0 || current.Low < snap.Low {
			snap.Low = current.Low
		}
		snap.DayVolume += current.Volume
	default:
		snap.PostMarketVolume += current.Volume
		snap.PostMarketClose = current.Close
	}
}
