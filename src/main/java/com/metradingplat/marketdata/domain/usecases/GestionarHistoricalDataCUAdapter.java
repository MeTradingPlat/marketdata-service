package com.metradingplat.marketdata.domain.usecases;

import java.time.Duration;
import java.time.Instant;
import java.time.OffsetDateTime;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.stream.Collectors;

import com.metradingplat.marketdata.application.input.GestionarHistoricalDataCUIntPort;
import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;

import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

@RequiredArgsConstructor
@Slf4j
public class GestionarHistoricalDataCUAdapter implements GestionarHistoricalDataCUIntPort {

    private final GestionarComunicacionExternalGatewayIntPort objExternalCommunicationGateway;

    @Override
    public List<Candle> getCandles(String symbol, EnumTimeframe timeframe, OffsetDateTime endDate, Integer bars) {
        List<Candle> allCandles = this.objExternalCommunicationGateway.getCandles(symbol, timeframe);

        if (allCandles == null || allCandles.isEmpty()) {
            return List.of();
        }

        Instant now = Instant.now();
        Instant effectiveEnd = (endDate != null) ? endDate.toInstant() : now;
        Duration candleDuration = timeframe.getDuration();

        // Filtrar: solo barras completas (cuyo periodo ya termino)
        List<Candle> completed = allCandles.stream()
                .filter(c -> {
                    Instant candleEnd = c.getTimestamp().plus(candleDuration);
                    return !candleEnd.isAfter(now)             // la barra ya cerro
                            && !candleEnd.isAfter(effectiveEnd); // antes del endDate
                })
                .collect(Collectors.toList());

        log.info("Candles para {} {}: {} totales, {} completas (endDate={}, bars={})",
                symbol, timeframe, allCandles.size(), completed.size(), endDate, bars);

        // Si se especifica bars, tomar las ultimas N
        if (bars != null && bars > 0 && bars < completed.size()) {
            completed = completed.subList(completed.size() - bars, completed.size());
        }

        return completed;
    }

    @Override
    public Map<String, List<Candle>> getCandlesBatch(List<String> symbols, EnumTimeframe timeframe, int bars) {
        log.info("Batch fetching {} symbols, timeframe={}, bars={}", symbols.size(), timeframe, bars);

        // Obtener datos brutos del gateway
        Map<String, List<Candle>> rawData = this.objExternalCommunicationGateway.getCandlesBatch(symbols, timeframe, bars);

        // Filtrar y procesar cada simbolo
        Map<String, List<Candle>> resultado = new HashMap<>();
        Instant now = Instant.now();
        Duration candleDuration = timeframe.getDuration();

        for (Map.Entry<String, List<Candle>> entry : rawData.entrySet()) {
            String symbol = entry.getKey();
            List<Candle> allCandles = entry.getValue();

            if (allCandles == null || allCandles.isEmpty()) {
                resultado.put(symbol, List.of());
                continue;
            }

            // Filtrar solo barras completas
            List<Candle> completed = allCandles.stream()
                .filter(c -> !c.getTimestamp().plus(candleDuration).isAfter(now))
                .collect(Collectors.toList());

            // Truncar a las ultimas N barras si es necesario
            if (bars > 0 && bars < completed.size()) {
                completed = completed.subList(completed.size() - bars, completed.size());
            }

            resultado.put(symbol, completed);
        }

        log.info("Batch complete: {} simbolos procesados, {} con datos",
            resultado.size(), resultado.values().stream().filter(l -> !l.isEmpty()).count());

        return resultado;
    }

    @Override
    public Candle getLastCandle(String symbol, EnumTimeframe timeframe) {
        List<Candle> candles = getCandles(symbol, timeframe, null, 1);
        return candles.isEmpty() ? null : candles.get(0);
    }

    @Override
    public Candle getCurrentCandle(String symbol, EnumTimeframe timeframe) {
        List<Candle> allCandles = this.objExternalCommunicationGateway.getCandles(symbol, timeframe);

        if (allCandles == null || allCandles.isEmpty()) {
            return null;
        }

        Instant now = Instant.now();
        Duration candleDuration = timeframe.getDuration();

        // Filtrar: solo la barra en formacion (cuyo periodo aun no ha cerrado)
        return allCandles.stream()
                .filter(c -> c.getTimestamp().plus(candleDuration).isAfter(now))
                .reduce((first, second) -> second) // la mas reciente
                .orElse(null);
    }
}
