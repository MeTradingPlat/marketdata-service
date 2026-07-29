package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Un canal Candle abierto dentro del pool, con hasta CAPACITY simbolos+timeframe
 * suscritos y su propio reloj de inactividad por simbolo (tocado tanto por
 * peticiones como por ticks en vivo, igual que el candleLastAccess original).
 */
class PooledCandleChannel {

    static final int CAPACITY = 100;

    final DxLinkClient.DxLinkChannel channel;
    private final ConcurrentHashMap<String, Long> symbols = new ConcurrentHashMap<>();

    PooledCandleChannel(DxLinkClient.DxLinkChannel channel) {
        this.channel = channel;
    }

    boolean hasRoom() { return symbols.size() < CAPACITY; }
    int occupancy() { return symbols.size(); }
    boolean isEmpty() { return symbols.isEmpty(); }
    boolean contains(String key) { return symbols.containsKey(key); }
    void touch(String key) { symbols.put(key, System.currentTimeMillis()); }
    void remove(String key) { symbols.remove(key); }
    Set<String> keys() { return symbols.keySet(); }
    long lastAccess(String key) { return symbols.getOrDefault(key, 0L); }
}
