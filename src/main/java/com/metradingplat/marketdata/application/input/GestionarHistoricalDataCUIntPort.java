package com.metradingplat.marketdata.application.input;

import java.time.OffsetDateTime;
import java.util.List;
import java.util.Map;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;

public interface GestionarHistoricalDataCUIntPort {
    List<Candle> getCandles(String symbol, EnumTimeframe timeframe, OffsetDateTime endDate, Integer bars);

    Map<String, List<Candle>> getCandlesBatch(List<String> symbols, EnumTimeframe timeframe, int bars);

    Candle getLastCandle(String symbol, EnumTimeframe timeframe);

    // Batch methods for single candle
    Map<String, Candle> getLastCandleBatch(List<String> symbols, EnumTimeframe timeframe);

    Candle getCurrentCandle(String symbol, EnumTimeframe timeframe);

    Map<String, Candle> getCurrentCandleBatch(List<String> symbols, EnumTimeframe timeframe);
}
