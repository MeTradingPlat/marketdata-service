package com.metradingplat.marketdata.application.output;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;

import java.util.List;

public interface GestionarCandleRepositoryIntPort {

    /**
     * Saves or updates a candle in the database (upsert by
     * symbol+timeframe+timestamp).
     */
    void upsert(Candle candle);

    /**
     * Returns the most recent candle stored for a given symbol and timeframe, or
     * null if none exists.
     */
    Candle findMostRecent(String symbol, EnumTimeframe timeframe);

    /**
     * Returns the N most recent candles for a given symbol and timeframe, sorted
     * ascending.
     */
    List<Candle> findLatest(String symbol, EnumTimeframe timeframe, int limit);

    /**
     * Elimina todas las candles cuyos timeframes NO estén en la lista indicada.
     * Usado para la limpieza diaria de timeframes efímeros.
     */
    void deleteByTimeframesNotIn(List<EnumTimeframe> timeframes);
}
