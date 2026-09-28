package tastytrade

// liveDayVolumeEntry: Regular es Trade.dayVolume (volumen oficial de sesion
// regular), Extended es TradeETH.dayVolumeAsDouble (SOLO pre/post-market,
// campo separado que dxFeed mantiene aparte -- ver trade_eth_event.go). Los
// dos se actualizan solos con cada evento, sin ciclos de suscribir/pedir/
// desuscribir -- reemplaza el snapshot puntual de FetchDayVolumes mientras
// el simbolo tenga su Trade+TradeETH en vivo (ver subscribeLiveDayVolume).
type liveDayVolumeEntry struct {
	Regular  *float64
	Extended *float64
}

// handleLiveTradeVolume/handleLiveTradeETHVolume: wireChannel las registra
// como el UNICO onTrade/onTradeETH de cada canal pooled (a diferencia de
// Candle, que necesita un dispatch por simbolo+timeframe para el
// tick/cierre de cada suscriptor -- ver candle_dispatch.go), Trade/TradeETH
// aca solo actualizan un valor en memoria por simbolo, sin ningun
// suscriptor que avisar.
func (p *CandlePool) handleLiveTradeVolume(ev rawTradeEvent) {
	if ev.DayVolume == nil {
		return
	}
	p.liveDayVolumeMu.Lock()
	defer p.liveDayVolumeMu.Unlock()
	entry := p.liveDayVolume[ev.Symbol]
	entry.Regular = ev.DayVolume
	p.liveDayVolume[ev.Symbol] = entry
}

func (p *CandlePool) handleLiveTradeETHVolume(ev rawTradeETHEvent) {
	if ev.ExtendedDayVolume == nil {
		return
	}
	p.liveDayVolumeMu.Lock()
	defer p.liveDayVolumeMu.Unlock()
	entry := p.liveDayVolume[ev.Symbol]
	entry.Extended = ev.ExtendedDayVolume
	p.liveDayVolume[ev.Symbol] = entry
}

// LiveDayVolume devuelve el ultimo volumen conocido en memoria para un
// simbolo -- ok es false si todavia no ha llegado ningun Trade suyo desde
// que se suscribio (recien arrancado, o simbolo sin operar hoy).
func (p *CandlePool) LiveDayVolume(symbol string) (entry liveDayVolumeEntry, ok bool) {
	p.liveDayVolumeMu.RLock()
	defer p.liveDayVolumeMu.RUnlock()
	entry, ok = p.liveDayVolume[symbol]
	return entry, ok
}

// LiveExtendedDayVolumes implementa out.DayVolumeGateway -- lectura pura de
// lo que ya esta en memoria, sin ningun I/O de red.
func (p *CandlePool) LiveExtendedDayVolumes(symbols []string) map[string]int64 {
	p.liveDayVolumeMu.RLock()
	defer p.liveDayVolumeMu.RUnlock()
	result := make(map[string]int64, len(symbols))
	for _, symbol := range symbols {
		if entry, ok := p.liveDayVolume[symbol]; ok && entry.Extended != nil {
			result[symbol] = int64(*entry.Extended)
		}
	}
	return result
}
