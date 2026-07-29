package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;

import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.concurrent.atomic.AtomicLong;

class CandleMergeListener {

    static DxLinkClient.CandleCallback forResult(EnumTimeframe timeframe, Map<String, List<Candle>> resultado,
            AtomicLong lastEventAt) {
        return (symbol, candle, isSnapshotComplete) -> {
            String cleanSymbol = symbol.replaceAll("\\{=.*\\}", "");
            candle.setSymbol(cleanSymbol);
            candle.setTimeframe(timeframe);
            List<Candle> list = resultado.computeIfAbsent(cleanSymbol, k -> new java.util.ArrayList<>());
            Optional<Candle> existing = list.stream()
                    .filter(c -> c.getTimestamp().equals(candle.getTimestamp())).findFirst();
            if (existing.isPresent()) {
                mergeInto(existing.get(), candle);
            } else {
                list.add(candle);
            }
            lastEventAt.set(System.currentTimeMillis());
        };
    }

    private static void mergeInto(Candle target, Candle incoming) {
        if (incoming.getHigh() != null && (target.getHigh() == null || incoming.getHigh() > target.getHigh())) target.setHigh(incoming.getHigh());
        if (incoming.getLow() != null && (target.getLow() == null || incoming.getLow() < target.getLow())) target.setLow(incoming.getLow());
        if (incoming.getClose() != null) target.setClose(incoming.getClose());
        if (incoming.getVolume() != null) target.setVolume(incoming.getVolume());
    }
}
