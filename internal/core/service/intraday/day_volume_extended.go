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

// SetExtendedBaseline congela el Extended de cada simbolo -- se llama desde
// el mismo loop que SetExtended (ver day_volume_extended_loop.go) pero SOLO
// mientras la fase no es post-market: en PreMarket/Regular sigue
// pisandose con el valor mas reciente (Extended no se mueve en Regular, asi
// que da lo mismo cual de esas llamadas sea "la ultima"), y en el instante
// en que la fase pasa a PostMarket, el loop deja de llamar esto -- el
// ultimo valor escrito queda congelado solo, sin ninguna ventana de
// segundos que acertar (a diferencia de la vieja foto puntual de
// RegularEnd via Trade.dayVolume).
func (t *DayVolumeTracker) SetExtendedBaseline(day time.Time, volumes map[string]int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.resetForDayLocked(day)
	for symbol, volume := range volumes {
		t.extendedBaseline[symbol] = volume
	}
}

// ExtendedBaseline es el Extended congelado al cierre de la sesion regular
// -- withRealSessionVolumes resta esto del Extended ACTUAL (que para
// entonces ya volvio a moverse, sumando volumen de post-market) para
// obtener el volumen de post-market puro, igual que preMarketVolume se
// deriva de Extended durante la sesion regular.
func (t *DayVolumeTracker) ExtendedBaseline(symbol string) (int64, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	v, ok := t.extendedBaseline[symbol]
	return v, ok
}
