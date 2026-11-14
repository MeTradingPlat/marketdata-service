package tastytrade

func (c *dxLinkChannel) handleData(data []interface{}) {
	i := 0
	for i < len(data) {
		typeName, ok := data[i].(string)
		if !ok {
			i++
			continue
		}
		if i+1 >= len(data) {
			i++
			continue
		}
		switch typeName {
		case "Candle":
			if batch, ok := data[i+1].([]interface{}); ok {
				for _, ev := range parseCandleBatch(batch) {
					c.dispatchCandle(ev)
				}
			}
		case "Profile":
			if batch, ok := data[i+1].([]interface{}); ok {
				for _, ev := range parseProfileBatch(batch) {
					c.dispatchProfile(ev)
				}
			}
		case "Trade":
			if batch, ok := data[i+1].([]interface{}); ok {
				for _, ev := range parseTradeBatch(batch) {
					c.dispatchTrade(ev)
				}
			}
		case "TradeETH":
			if batch, ok := data[i+1].([]interface{}); ok {
				for _, ev := range parseTradeETHBatch(batch) {
					c.dispatchTradeETH(ev)
				}
			}
		}
		i += 2
	}
}

func (c *dxLinkChannel) setOnCandle(fn func(rawCandleEvent)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onCandle = fn
}

func (c *dxLinkChannel) dispatchCandle(ev rawCandleEvent) {
	c.mu.RLock()
	fn := c.onCandle
	c.mu.RUnlock()
	if fn != nil {
		fn(ev)
	}
}

func (c *dxLinkChannel) setOnProfile(fn func(rawProfileEvent)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onProfile = fn
}

func (c *dxLinkChannel) dispatchProfile(ev rawProfileEvent) {
	c.mu.RLock()
	fn := c.onProfile
	c.mu.RUnlock()
	if fn != nil {
		fn(ev)
	}
}

func (c *dxLinkChannel) setOnTrade(fn func(rawTradeEvent)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onTrade = fn
}

func (c *dxLinkChannel) dispatchTrade(ev rawTradeEvent) {
	c.mu.RLock()
	fn := c.onTrade
	c.mu.RUnlock()
	if fn != nil {
		fn(ev)
	}
}

func (c *dxLinkChannel) setOnTradeETH(fn func(rawTradeETHEvent)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onTradeETH = fn
}

func (c *dxLinkChannel) dispatchTradeETH(ev rawTradeETHEvent) {
	c.mu.RLock()
	fn := c.onTradeETH
	c.mu.RUnlock()
	if fn != nil {
		fn(ev)
	}
}
