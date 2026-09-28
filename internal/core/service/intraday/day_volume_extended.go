package intraday

import "time"

// SetExtended reemplaza el volumen extendido (TradeETH.dayVolumeAsDouble)
// conocido de cada simbolo -- llamado periodicamente con lo que ya esta en
// memoria del lado del adaptador (ver day_volume_extended_loop.go), no un
// fetch de red, asi que puede llamarse seguido sin costo.
func (t *DayVolumeTracker) SetExtended(day time.Time, volumes map[string]int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.resetForDayLocked(day)
	for symbol, volume := range volumes {
		t.extended[symbol] = volume
	}
}

// Extended devuelve el ultimo volumen de horario extendido conocido -- ver
// withRealSessionVolumes: durante sesion regular y post-market, este valor
// YA ES el volumen de pre-market (TradeETH no se mueve hasta las 16:00),
// sin necesidad de la foto puntual de preMarketEnd.
func (t *DayVolumeTracker) Extended(symbol string) (int64, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	v, ok := t.extended[symbol]
	return v, ok
}
