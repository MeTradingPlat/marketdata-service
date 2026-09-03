package dto

import "github.com/MeTradingPlat/marketdata-service/internal/core/domain"

// SnapshotMessage lleva la sesion intradia completa del simbolo (OHLC,
// volumenes pre/regular/post, precio y volumen actuales) -- se manda cada
// vez que cierra una M1 nueva del simbolo (misma cadencia que /ws/candles),
// no con cada tick intra-minuto: el snapshot ya resume el minuto entero, no
// hace falta mas frecuencia que esa para que el frontend vea el dato
// fresco.
type SnapshotMessage struct {
	Type     string                  `json:"type"`
	Symbol   string                  `json:"symbol"`
	Snapshot domain.IntradaySnapshot `json:"snapshot"`
}

type SnapshotControlMessage struct {
	Type    string `json:"type"`
	Symbol  string `json:"symbol,omitempty"`
	Message string `json:"message,omitempty"`
}
