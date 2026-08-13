package com.metradingplat.marketdata.infrastructure.output.external.gateway;

import java.util.List;
import java.util.Map;
import java.util.function.Consumer;

import org.springframework.stereotype.Service;

import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.ActiveEquity;
import com.metradingplat.marketdata.domain.models.BracketOrder;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.domain.models.OrderRequest;
import com.metradingplat.marketdata.domain.models.OrderResponse;
import com.metradingplat.marketdata.domain.models.FundamentalData;
import com.metradingplat.marketdata.infrastructure.output.external.tastytrade.TastyTradeService;

import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

@Service
@RequiredArgsConstructor
@Slf4j
public class GestionarComunicacionExternalGatewayImplAdapter implements GestionarComunicacionExternalGatewayIntPort {

    private final TastyTradeService tastyTradeService;

    @Override
    public void sendOrder(OrderRequest request) {
        log.info("Gateway: Sending order for symbol: {}", request.getSymbol());
        tastyTradeService.sendOrder(request);
    }

    @Override
    public OrderResponse sendBracketOrder(BracketOrder order) {
        log.info("Gateway: Sending bracket order for symbol: {}", order.getSymbol());
        return tastyTradeService.sendBracketOrder(order);
    }

    @Override
    public void cancelOrder(String orderId) {
        log.info("Gateway: Cancelling order: {}", orderId);
        tastyTradeService.cancelOrder(orderId);
    }

    @Override
    public List<Candle> getCandles(String symbol, EnumTimeframe timeframe) {
        log.debug("Gateway: Fetching candles for symbol: {} timeframe: {}", symbol, timeframe);
        return tastyTradeService.getCandles(symbol, timeframe);
    }

    @Override
    public List<Candle> probeMaxDepth(String symbol, EnumTimeframe timeframe) {
        log.debug("Gateway: Probing max depth for symbol: {} timeframe: {}", symbol, timeframe);
        return tastyTradeService.probeMaxDepth(symbol, timeframe);
    }

    @Override
    public Map<String, List<Candle>> getCandlesBatch(List<String> symbols, EnumTimeframe timeframe, int bars) {
        log.debug("Gateway: Batch fetching candles for {} symbols, timeframe: {}, bars: {}", symbols.size(), timeframe,
                bars);
        return tastyTradeService.getCandlesBatch(symbols, timeframe, bars);
    }

    @Override
    public Map<String, List<Candle>> getLastCandleBatch(List<String> symbols, EnumTimeframe timeframe) {
        log.debug("Gateway: Batch fetching LAST candle for {} symbols, timeframe: {}", symbols.size(), timeframe);
        // Pedimos 50 barras sin cache para asegurar tener la ultima cerrada
        return tastyTradeService.getCandlesBatchNoCache(symbols, timeframe, 50);
    }

    @Override
    public Map<String, List<Candle>> getCurrentCandleBatch(List<String> symbols, EnumTimeframe timeframe) {
        log.debug("Gateway: Batch fetching CURRENT candle for {} symbols, timeframe: {}", symbols.size(), timeframe);
        // Pedimos 10 barras sin cache para tener la barra en formacion
        return tastyTradeService.getCandlesBatchNoCache(symbols, timeframe, 10);
    }

    @Override
    public void addCandleLiveListener(String symbol, EnumTimeframe timeframe, Consumer<Candle> listener) {
        tastyTradeService.addCandleLiveListener(symbol, timeframe, listener);
    }

    @Override
    public void removeCandleLiveListener(String symbol, EnumTimeframe timeframe, Consumer<Candle> listener) {
        tastyTradeService.removeCandleLiveListener(symbol, timeframe, listener);
    }

    @Override
    public List<ActiveEquity> getActiveEquities(int pageOffset, int perPage) {
        log.info("Gateway: Fetching active equities page={} perPage={}", pageOffset, perPage);
        return tastyTradeService.getActiveEquities(pageOffset, perPage);
    }

    @Override
    public Map<String, Object> getMarketDataByType(String symbol) {
        log.info("Gateway: Fetching market data for symbol: {}", symbol);
        return tastyTradeService.getMarketDataByType(symbol);
    }

    @Override
    public List<Map<String, Object>> getEarningsReports(String symbol, String startDate) {
        log.info("Gateway: Fetching earnings for symbol: {} from {}", symbol, startDate);
        return tastyTradeService.getEarningsReports(symbol, startDate);
    }

    @Override
    public Map<String, FundamentalData> getFundamentalsBatch(List<String> symbols) {
        log.info("Gateway: Batch fetching fundamentals for {} symbols", symbols.size());
        return tastyTradeService.getFundamentalsBatch(symbols);
    }

    @Override
    public List<Map<String, Object>> getMarketMetricsBatch(List<String> symbols) {
        log.info("Gateway: Batch fetching market metrics for {} symbols", symbols.size());
        return tastyTradeService.getMarketMetricsBatch(symbols);
    }
}
