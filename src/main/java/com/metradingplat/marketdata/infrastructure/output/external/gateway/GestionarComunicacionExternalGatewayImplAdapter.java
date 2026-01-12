package com.metradingplat.marketdata.infrastructure.output.external.gateway;

import java.time.OffsetDateTime;
import java.util.ArrayList;
import java.util.List;

import org.springframework.stereotype.Service;

import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.domain.models.OrderRequest;

import lombok.extern.slf4j.Slf4j;

@Service
@Slf4j
public class GestionarComunicacionExternalGatewayImplAdapter implements GestionarComunicacionExternalGatewayIntPort {

    @Override
    public void sendOrder(OrderRequest request) {
        log.info("Sending order for symbol: {}", request.getSymbol());
        // TODO: Implement Tastytrade order logic
    }

    @Override
    public void subscribe(String symbol) {
        log.info("Subscribing to real-time data for symbol: {}", symbol);
        // TODO: Implement DxLink subscription
    }

    @Override
    public void unsubscribe(String symbol) {
        log.info("Unsubscribing from real-time data for symbol: {}", symbol);
        // TODO: Implement DxLink unsubscription
    }

    @Override
    public List<Candle> getCandles(String symbol, EnumTimeframe timeframe, OffsetDateTime from, OffsetDateTime to) {
        log.info("Fetching candles for symbol: {} from {} to {}", symbol, from, to);
        // TODO: Implement Tastytrade historical data fetching
        return new ArrayList<>();
    }
}
