package com.metradingplat.marketdata.domain.usecases;

import java.time.OffsetDateTime;
import java.time.ZoneOffset;
import java.time.temporal.ChronoUnit;
import java.util.List;

import com.metradingplat.marketdata.application.input.GestionarHistoricalDataCUIntPort;
import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;

import lombok.RequiredArgsConstructor;

/**
 * Caso de uso para obtener datos históricos.
 * Los datos se obtienen directamente de DxLink sin caché en BD.
 */
@RequiredArgsConstructor
public class GestionarHistoricalDataCUAdapter implements GestionarHistoricalDataCUIntPort {

    private final GestionarComunicacionExternalGatewayIntPort objExternalCommunicationGateway;

    @Override
    public List<Candle> getHistoricalMarketData(String symbol, EnumTimeframe timeframe, OffsetDateTime from,
            OffsetDateTime to) {
        return this.objExternalCommunicationGateway.getCandles(symbol, timeframe, from, to);
    }

    @Override
    public Candle getLastCandle(String symbol, EnumTimeframe timeframe) {
        OffsetDateTime now = OffsetDateTime.now(ZoneOffset.UTC);
        OffsetDateTime from = calcularInicioRango(timeframe, now);

        List<Candle> candles = this.objExternalCommunicationGateway.getCandles(symbol, timeframe, from, now);

        if (candles == null || candles.isEmpty()) {
            return null;
        }

        return candles.get(candles.size() - 1);
    }

    private OffsetDateTime calcularInicioRango(EnumTimeframe timeframe, OffsetDateTime now) {
        return switch (timeframe) {
            case M1 -> now.minus(2, ChronoUnit.MINUTES);
            case M5 -> now.minus(10, ChronoUnit.MINUTES);
            case M15 -> now.minus(30, ChronoUnit.MINUTES);
            case M30 -> now.minus(1, ChronoUnit.HOURS);
            case H1 -> now.minus(2, ChronoUnit.HOURS);
            case D1 -> now.minus(2, ChronoUnit.DAYS);
            case W1 -> now.minus(2, ChronoUnit.WEEKS);
            case MO1 -> now.minus(2, ChronoUnit.MONTHS);
        };
    }
}
