package com.metradingplat.marketdata.application.input;

import java.time.OffsetDateTime;
import java.util.List;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;

public interface GestionarHistoricalDataCUIntPort {
    List<Candle> getCandles(String symbol, EnumTimeframe timeframe, OffsetDateTime endDate, Integer bars);

    Candle getLastCandle(String symbol, EnumTimeframe timeframe);
}
