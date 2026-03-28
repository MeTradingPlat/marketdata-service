package com.metradingplat.marketdata.domain.usecases;

import java.util.List;
import java.util.Map;

import com.metradingplat.marketdata.application.input.GestionarQuoteCUIntPort;
import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.models.Quote;

import lombok.RequiredArgsConstructor;

@RequiredArgsConstructor
public class GestionarQuoteCUAdapter implements GestionarQuoteCUIntPort {

    private final GestionarComunicacionExternalGatewayIntPort objExternalGateway;

    @Override
    @SuppressWarnings("unchecked")
    public Quote obtenerQuote(String symbol) {
        Map<String, Object> data = this.objExternalGateway.getMarketDataByType(symbol);
        if (data == null || data.isEmpty()) {
            return Quote.builder().symbol(symbol).build();
        }

        // TastyTrade retorna items dentro de data
        List<Map<String, Object>> items = (List<Map<String, Object>>) data.get("items");
        if (items == null || items.isEmpty()) {
            return Quote.builder().symbol(symbol).build();
        }

        Map<String, Object> item = items.get(0);

        return Quote.builder()
                .symbol(symbol)
                .bid(toDouble(item.get("bid")))
                .ask(toDouble(item.get("ask")))
                .last(toDouble(item.get("last")))
                .open(toDouble(item.get("open")))
                .high(toDouble(item.get("day-high-price")))
                .low(toDouble(item.get("day-low-price")))
                .close(toDouble(item.get("close")))
                .prevClose(toDouble(item.get("prev-close")))
                .volume(toDouble(item.get("volume")))
                .tradingHalted(item.get("is-trading-halted") != null ? (Boolean) item.get("is-trading-halted") : false)
                .tradingHaltedReason(item.get("halt-reason") != null ? item.get("halt-reason").toString() : null)
                .beta(toDouble(item.get("beta")))
                .build();
    }

    private Double toDouble(Object value) {
        if (value == null) return 0.0;
        if (value instanceof Number) return ((Number) value).doubleValue();
        try {
            return Double.parseDouble(value.toString());
        } catch (NumberFormatException e) {
            return 0.0;
        }
    }
}
