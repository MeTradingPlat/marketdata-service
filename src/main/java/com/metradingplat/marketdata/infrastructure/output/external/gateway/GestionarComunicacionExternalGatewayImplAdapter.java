package com.metradingplat.marketdata.infrastructure.output.external.gateway;

import java.time.OffsetDateTime;
import java.util.List;
import java.util.Map;

import org.springframework.stereotype.Service;

import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.ActiveEquity;
import com.metradingplat.marketdata.domain.models.BracketOrder;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.domain.models.OrderRequest;
import com.metradingplat.marketdata.domain.models.OrderResponse;
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
    public void subscribe(String symbol) {
        log.info("Gateway: Subscribing to real-time data for symbol: {}", symbol);
        tastyTradeService.subscribe(symbol);
    }

    @Override
    public void unsubscribe(String symbol) {
        log.info("Gateway: Unsubscribing from real-time data for symbol: {}", symbol);
        tastyTradeService.unsubscribe(symbol);
    }

    @Override
    public List<Candle> getCandles(String symbol, EnumTimeframe timeframe, OffsetDateTime from, OffsetDateTime to) {
        log.info("Gateway: Fetching candles for symbol: {} from {} to {}", symbol, from, to);
        return tastyTradeService.getCandles(symbol, timeframe, from, to);
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
}
