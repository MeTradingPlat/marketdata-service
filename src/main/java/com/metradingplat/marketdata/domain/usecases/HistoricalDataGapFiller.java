package com.metradingplat.marketdata.domain.usecases;

import java.time.Instant;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;

import com.metradingplat.marketdata.application.output.GestionarHistoricalDataServiceGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;

import lombok.RequiredArgsConstructor;

/**
 * Cuando DxLink no alcanza a cubrir los "bars" pedidos (su ventana en vivo es
 * limitada, sobre todo en timeframes de minutos), rellena el resto pidiendo
 * las barras mas antiguas a historical-data-service. El merge es puramente
 * local a esta llamada -- no toca el cache propio de DxLink/TastyTradeService,
 * asi que no se "engorda" el cache con historial profundo que casi nunca se
 * vuelve a pedir.
 */
@RequiredArgsConstructor
public class HistoricalDataGapFiller {

    private final GestionarHistoricalDataServiceGatewayIntPort historicalDataGateway;

    public List<Candle> fill(List<Candle> dxLinkCandles, String symbol, EnumTimeframe timeframe, Integer barsWanted,
            Instant anchorEnd) {
        if (barsWanted == null || barsWanted <= 0 || dxLinkCandles.size() >= barsWanted) {
            return dxLinkCandles;
        }

        int missing = barsWanted - dxLinkCandles.size();
        // Si DxLink no tiene NINGUNA barra en el rango pedido (ej. el usuario ya
        // se desplazo mas atras de lo que DxLink puede servir), no hay que
        // anclar en "ahora" -- eso traeria barras recientes que no van ahi y
        // rompe el orden estricto que exige el grafico. Se ancla en el
        // "endDate" que pidio el llamador.
        Instant oldestKnown = dxLinkCandles.stream()
                .map(Candle::getTimestamp)
                .min(Comparator.naturalOrder())
                .orElse(anchorEnd);

        List<Candle> older = historicalDataGateway.getCandlesBefore(symbol, timeframe, oldestKnown, missing);
        if (older.isEmpty()) {
            return dxLinkCandles;
        }

        List<Candle> merged = new ArrayList<>(older);
        merged.addAll(dxLinkCandles);
        merged.sort(Comparator.comparing(Candle::getTimestamp));
        return merged;
    }
}
