package com.metradingplat.marketdata.domain.usecases;

import java.time.Duration;
import java.time.OffsetDateTime;
import java.util.List;

import com.metradingplat.marketdata.application.input.GestionarHistoricalDataCUIntPort;
import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.application.output.GestionarHistoricalDataGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;

import lombok.RequiredArgsConstructor;

@RequiredArgsConstructor
public class GestionarHistoricalDataCUAdapter implements GestionarHistoricalDataCUIntPort {

    private final GestionarHistoricalDataGatewayIntPort objObtenerHistoricalDataGateway;
    private final GestionarComunicacionExternalGatewayIntPort objExternalCommunicationGateway;

    @Override
    public List<Candle> getHistoricalMarketData(String symbol, EnumTimeframe timeframe, OffsetDateTime from,
            OffsetDateTime to) {

        long count = this.objObtenerHistoricalDataGateway.countData(symbol, timeframe, from, to);
        long expected = calculateExpectedCandles(timeframe, from, to);

        // If data is incomplete or missing, fetch from external and save
        if (count < expected) {
            List<Candle> externalCandles = this.objExternalCommunicationGateway.getCandles(symbol, timeframe, from, to);
            if (!externalCandles.isEmpty()) {
                this.saveCandles(externalCandles);
                return externalCandles;
            }
        }

        return this.objObtenerHistoricalDataGateway.getHistoricalData(symbol, timeframe, from, to);
    }

    private long calculateExpectedCandles(EnumTimeframe timeframe, OffsetDateTime from, OffsetDateTime to) {
        long minutes = Duration.between(from, to).toMinutes();
        int timeframeMinutes = switch (timeframe) {
            case M1 -> 1;
            case M5 -> 5;
            case M15 -> 15;
            case H1 -> 60;
            case D1 -> 1440;
            default -> 1;
        };
        return minutes / timeframeMinutes;
    }

    @Override
    public void saveCandles(List<Candle> candles) {
        this.objObtenerHistoricalDataGateway.saveCandles(candles);
    }
}
