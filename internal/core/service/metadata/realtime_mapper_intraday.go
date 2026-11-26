package metadata

import (
	"strconv"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
)

// applyIntraday siempre corre (nunca hay un "nunca se calculo": GetSnapshot
// no falla salvo error de BD, y en ese caso llega con el zero-value) --
// dayVolume/preMarketVolume/postMarketVolume en 0 es un dato real (sin
// sesion hoy, ej. fin de semana), no ausencia de calculo.
func applyIntraday(out *dto.FundamentalRealtime, snap domain.IntradaySnapshot) {
	dayVolume := snap.DayVolume
	out.DayVolume = &dayVolume
	preVol := snap.PreMarketVolume
	out.PreMarketVolume = &preVol
	postVol := snap.PostMarketVolume
	out.PostMarketVolume = &postVol

	out.PrevClose = nonzeroOrNil(snap.PrevClose)
	out.Open = nonzeroOrNil(snap.Open)
	out.High = nonzeroOrNil(snap.High)
	out.Low = nonzeroOrNil(snap.Low)
	out.PreMarketClose = nonzeroOrNil(snap.PreMarketClose)
	out.PostMarketClose = nonzeroOrNil(snap.PostMarketClose)
	out.LastPrice = nonzeroOrNil(snap.CurrentPrice)
}

func applyDividendAndHalt(out *dto.FundamentalRealtime, f domain.Fundamentals) {
	amount := f.DividendAmount
	out.DividendAmount = &amount
	freq := strconv.FormatFloat(f.DividendFrequency, 'f', -1, 64)
	out.DividendFrequency = &freq
	if f.TradingStatus != "" {
		status := f.TradingStatus
		out.TradingStatus = &status
	}
	if f.HaltStartTime >= 0 {
		out.HaltStartTime = &f.HaltStartTime
	}
	if f.HaltEndTime >= 0 {
		out.HaltEndTime = &f.HaltEndTime
	}
}
