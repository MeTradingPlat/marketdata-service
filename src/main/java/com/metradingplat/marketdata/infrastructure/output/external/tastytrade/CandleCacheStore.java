package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.Collections;
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

    // Una vela "shell" -- llego un evento con timestamp pero sin OHLC todavia
    // completo (fragmento de un snapshot en curso) -- no cuenta como dato real
    // ni para decidir si ya se puede responder (awaitHistory) ni para lo que
    // se le entrega a un caller: un escaner evaluando patrones de velas sobre
    // un OHLC en null puede generar una senal falsa, es peor que esperar mas
    // o devolver menos velas de las pedidas.
    private static boolean isComplete(Candle c) {
        return c.getTimestamp() != null && c.getOpen() != null && c.getHigh() != null
                && c.getLow() != null && c.getClose() != null;
    }

    public boolean hasData(String key) {
        List<Candle> list = cache.get(key);
        if (list == null || list.isEmpty()) return false;
        synchronized (list) {
            return list.stream().anyMatch(CandleCacheStore::isComplete);
        }
    }

    public List<Candle> get(String key, int bars) {
        List<Candle> cached = cache.get(key);
        if (cached == null) return List.of();
        // El listener en vivo muta esta misma lista (merge(), en otro hilo) --
        // sincronizar sobre la instancia es obligatorio incluso leyendo via
        // stream, o corre riesgo de ConcurrentModificationException a mitad de
        // una peticion HTTP (visto en vivo: reventaba justo aqui).
        synchronized (cached) {
            if (cached.isEmpty()) return List.of();
            List<Candle> sorted = cached.stream()
                    .filter(CandleCacheStore::isComplete)
                    .sorted(java.util.Comparator.comparing(Candle::getTimestamp))
                    .toList();
            if (sorted.size() > bars) sorted = sorted.subList(sorted.size() - bars, sorted.size());
            return sorted;
        }
    }

    public Candle merge(String key, Candle incoming) {
        List<Candle> list = cache.computeIfAbsent(key, k -> Collections.synchronizedList(new ArrayList<>()));
        synchronized (list) {
            Optional<Candle> existing = list.stream()
                    .filter(c -> c.getTimestamp().equals(incoming.getTimestamp())).findFirst();
            if (existing.isPresent()) {
                Candle merged = existing.get();
                // El open de una vela se fija una sola vez, al primer tick de
                // esa barra -- si la primera vez que se creo esta vela vino
                // sin open (evento parcial), sin esto se quedaba en null para
                // siempre, aunque llegaran mas actualizaciones despues.
                if (merged.getOpen() == null && incoming.getOpen() != null) merged.setOpen(incoming.getOpen());
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
    }

    public void remove(String key) {
        cache.remove(key);
    }
}
