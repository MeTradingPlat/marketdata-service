package com.metradingplat.marketdata.application.output;

import java.util.List;
import java.util.Map;
import java.util.function.Consumer;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.ActiveEquity;
import com.metradingplat.marketdata.domain.models.BracketOrder;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.domain.models.FundamentalData;
import com.metradingplat.marketdata.domain.models.OrderRequest;
import com.metradingplat.marketdata.domain.models.OrderResponse;

public interface GestionarComunicacionExternalGatewayIntPort {
    void sendOrder(OrderRequest request);

    OrderResponse sendBracketOrder(BracketOrder order);

    void cancelOrder(String orderId);

    List<Candle> getCandles(String symbol, EnumTimeframe timeframe);

    Map<String, List<Candle>> getCandlesBatch(List<String> symbols, EnumTimeframe timeframe, int bars);

    Map<String, List<Candle>> getLastCandleBatch(List<String> symbols, EnumTimeframe timeframe);

    Map<String, List<Candle>> getCurrentCandleBatch(List<String> symbols, EnumTimeframe timeframe);

    void addCandleLiveListener(String symbol, EnumTimeframe timeframe, Consumer<Candle> listener);

    void removeCandleLiveListener(String symbol, EnumTimeframe timeframe, Consumer<Candle> listener);

    List<ActiveEquity> getActiveEquities(int pageOffset, int perPage);

    Map<String, Object> getMarketDataByType(String symbol);

    List<Map<String, Object>> getEarningsReports(String symbol, String startDate);

    Map<String, FundamentalData> getFundamentalsBatch(List<String> symbols);
    List<Map<String, Object>> getMarketMetricsBatch(List<String> symbols);
}
