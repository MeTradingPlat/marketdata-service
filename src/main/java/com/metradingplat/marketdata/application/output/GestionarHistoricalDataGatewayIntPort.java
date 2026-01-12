package com.metradingplat.marketdata.application.output;

import java.time.OffsetDateTime;
import java.util.List;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;

public interface GestionarHistoricalDataGatewayIntPort {
    List<Candle> getHistoricalData(String symbol, EnumTimeframe timeframe, OffsetDateTime from, OffsetDateTime to);

    long countData(String symbol, EnumTimeframe timeframe, OffsetDateTime from, OffsetDateTime to);

    void saveCandles(List<Candle> candles);
}
