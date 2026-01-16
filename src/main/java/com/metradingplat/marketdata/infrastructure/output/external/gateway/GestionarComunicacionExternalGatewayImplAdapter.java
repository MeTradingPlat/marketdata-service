package com.metradingplat.marketdata.infrastructure.output.external.gateway;

import java.time.OffsetDateTime;
import java.util.List;

import org.springframework.stereotype.Service;

import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.domain.models.OrderRequest;
import com.metradingplat.marketdata.infrastructure.output.external.tastytrade.TastyTradeService;

import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

/**
 * Adapter que implementa el gateway de comunicación externa.
 * Actúa como punto de entrada único para toda comunicación con exchanges externos.
 *
 * Actualmente delega a TastyTradeService. Si en el futuro se cambia de exchange,
 * solo es necesario cambiar la implementación interna de este adapter.
 */
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
}
