package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import com.metradingplat.marketdata.domain.models.Candle;
import org.springframework.stereotype.Component;

import java.util.List;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.function.Consumer;

/**
 * Pub/sub de velas en vivo por simbolo+timeframe -- usado por CandleWebSocketHandler
 * para reenviar cada tick a las sesiones suscritas, sin acoplar CandleSubscriptionPool
 * a WebSocket.
 */
@Component
public class CandleLiveListenerRegistry {

    private final ConcurrentHashMap<String, List<Consumer<Candle>>> listeners = new ConcurrentHashMap<>();

    public void add(String key, Consumer<Candle> listener) {
        listeners.computeIfAbsent(key, k -> new CopyOnWriteArrayList<>()).add(listener);
    }

    public void remove(String key, Consumer<Candle> listener) {
        List<Consumer<Candle>> list = listeners.get(key);
        if (list == null) return;
        list.remove(listener);
        if (list.isEmpty()) listeners.remove(key);
    }

    public void notify(String key, Candle candle) {
        List<Consumer<Candle>> list = listeners.get(key);
        if (list == null) return;
        for (Consumer<Candle> listener : list) listener.accept(candle);
    }
}
