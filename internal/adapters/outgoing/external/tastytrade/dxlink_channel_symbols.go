package tastytrade

import (
	"fmt"
	"strings"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// parseWireSymbol es el inverso de candleSymbol -- separa el simbolo
// desnudo y la temporalidad de lo que llega en un evento crudo. El
// dispatcher central del pool lo usa para saber a cual suscripcion (en
// vivo o historial) pertenece cada evento, ya que un canal pooled puede
// estar sirviendo varias a la vez.
//
// dxFeed omite el "1" de periodo en el simbolo que devuelve para velas de
// periodo 1 -- nos suscribimos con "AAPL{=1d}" pero el FEED_DATA que llega
// trae "AAPL{=d}" (confirmado en vivo: con solo el formato "{=1d}" el
// dispatcher nunca encontraba destinatario y todo se perdia en silencio
// hasta el timeout). Hay que aceptar ambas formas.
func parseWireSymbol(wire string) (symbol string, tf domain.Timeframe, ok bool) {
	idx := strings.Index(wire, "{=")
	if idx < 0 {
		return "", "", false
	}
	switch wire[idx:] {
	case "{=1m}", "{=m}":
		return wire[:idx], domain.M1, true
	case "{=1h}", "{=h}":
		return wire[:idx], domain.H1, true
	case "{=1d}", "{=d}":
		return wire[:idx], domain.D1, true
	default:
		return "", "", false
	}
}

func dxLinkCandleSuffix(tf domain.Timeframe) (string, error) {
	switch tf {
	case domain.M1:
		return "{=1m}", nil
	case domain.H1:
		return "{=1h}", nil
	case domain.D1:
		return "{=1d}", nil
	default:
		return "", fmt.Errorf("unsupported timeframe for dxlink: %s", tf)
	}
}

func candleSymbol(symbol string, tf domain.Timeframe) (string, error) {
	suffix, err := dxLinkCandleSuffix(tf)
	if err != nil {
		return "", err
	}
	return symbol + suffix, nil
}

// dxLinkCandleSuffixNormalized es la forma sin el "1" que dxFeed usa al
// devolver eventos de candles de periodo 1 (ver parseWireSymbol).
func dxLinkCandleSuffixNormalized(tf domain.Timeframe) (string, error) {
	switch tf {
	case domain.M1:
		return "{=m}", nil
	case domain.H1:
		return "{=h}", nil
	case domain.D1:
		return "{=d}", nil
	default:
		return "", fmt.Errorf("unsupported timeframe for dxlink: %s", tf)
	}
}

func candleSymbolNormalized(symbol string, tf domain.Timeframe) (string, error) {
	suffix, err := dxLinkCandleSuffixNormalized(tf)
	if err != nil {
		return "", err
	}
	return symbol + suffix, nil
}
