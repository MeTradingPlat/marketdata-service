package com.metradingplat.marketdata.infrastructure.output.external.historicaldata;

import java.time.Instant;
import java.util.List;

import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

import com.metradingplat.marketdata.application.output.GestionarHistoricalDataServiceGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;

import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

@Slf4j
@Component
@RequiredArgsConstructor
public class HistoricalDataServiceGatewayAdapter implements GestionarHistoricalDataServiceGatewayIntPort {

    private final RestClient historicalDataServiceRestClient;

    @Override
    public List<Candle> getCandlesBefore(String symbol, EnumTimeframe timeframe, Instant before, int maxBars) {
        try {
            HistoricalCandleDTO[] response = historicalDataServiceRestClient.get()
                    .uri(uriBuilder -> uriBuilder.path("/historical/candles")
                            .queryParam("symbol", symbol)
                            .queryParam("timeframe", timeframe.name())
                            .queryParam("bars", maxBars)
                            .queryParam("before", before.toString())
                            .build())
                    .retrieve()
                    .body(HistoricalCandleDTO[].class);

            if (response == null) {
                return List.of();
            }
            return List.of(response).stream().map(dto -> toCandle(dto, symbol, timeframe)).toList();
        } catch (Exception e) {
            log.warn("historical-data-service fallback failed for {} {}: {}", symbol, timeframe, e.getMessage());
            return List.of();
        }
    }

    private Candle toCandle(HistoricalCandleDTO dto, String symbol, EnumTimeframe timeframe) {
        return Candle.builder()
                .symbol(symbol)
                .timeframe(timeframe)
                .timestamp(dto.getTimestamp())
                .open(dto.getOpen())
                .high(dto.getHigh())
                .low(dto.getLow())
                .close(dto.getClose())
                .volume(dto.getVolume())
                .vwap(dto.getVwap())
                .build();
    }
}
