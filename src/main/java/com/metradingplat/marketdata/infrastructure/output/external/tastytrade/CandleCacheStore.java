package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.List;
import java.util.Optional;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Cache compartida de velas por simbolo+timeframe, alimentada en vivo por
 * CandleSubscriptionPool. El reloj de inactividad vive en PooledCandleChannel,
 * no aqui -- este store solo guarda y mergea datos.
 */
@Component
public class CandleCacheStore {

    private final ConcurrentHashMap<String, List<Candle>> cache = new ConcurrentHashMap<>();

    public static String key(String symbol, EnumTimeframe timeframe) {
        return symbol.toUpperCase() + "|" + timeframe.name();
    }

    public boolean hasData(String key) {
        List<Candle> list = cache.get(key);
        return list != null && !list.isEmpty();
    }

    public List<Candle> get(String key, int bars) {
        List<Candle> cached = cache.get(key);
        if (cached == null || cached.isEmpty()) return List.of();
        List<Candle> sorted = cached.stream()
                .filter(c -> c.getTimestamp() != null)
                .sorted(java.util.Comparator.comparing(Candle::getTimestamp))
                .toList();
        if (sorted.size() > bars) sorted = sorted.subList(sorted.size() - bars, sorted.size());
        return sorted;
    }

    public Candle merge(String key, Candle incoming) {
        List<Candle> list = cache.computeIfAbsent(key, k -> new ArrayList<>());
        Optional<Candle> existing = list.stream()
                .filter(c -> c.getTimestamp().equals(incoming.getTimestamp())).findFirst();
        if (existing.isPresent()) {
            Candle merged = existing.get();
            if (incoming.getHigh() != null && (merged.getHigh() == null || incoming.getHigh() > merged.getHigh())) merged.setHigh(incoming.getHigh());
            if (incoming.getLow() != null && (merged.getLow() == null || incoming.getLow() < merged.getLow())) merged.setLow(incoming.getLow());
            if (incoming.getClose() != null) merged.setClose(incoming.getClose());
            if (incoming.getVolume() != null) merged.setVolume(incoming.getVolume());
            return merged;
        }
        if (list.size() >= 2000) list.remove(0);
        list.add(incoming);
        return incoming;
    }

    public void remove(String key) {
        cache.remove(key);
    }
}
