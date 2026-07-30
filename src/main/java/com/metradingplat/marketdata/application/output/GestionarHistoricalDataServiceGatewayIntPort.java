package com.metradingplat.marketdata.application.output;

import java.time.Instant;
import java.util.List;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;

public interface GestionarHistoricalDataServiceGatewayIntPort {
    List<Candle> getCandlesBefore(String symbol, EnumTimeframe timeframe, Instant before, int maxBars);
}
