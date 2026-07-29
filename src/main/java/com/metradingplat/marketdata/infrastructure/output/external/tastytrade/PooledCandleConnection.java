package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;

/**
 * Una conexion DxLink persistente y dedicada del pool de velas, con sus
 * canales Candle abiertos (techo empirico medido esta sesion: MAX_CHANNELS
 * canales nuevos confiables por conexion).
 */
class PooledCandleConnection {

    static final int MAX_CHANNELS = 8;

    final DxLinkClient client;
    final List<PooledCandleChannel> channels = new CopyOnWriteArrayList<>();

    PooledCandleConnection(DxLinkClient client) {
        this.client = client;
    }

    boolean hasRoomForNewChannel() { return channels.size() < MAX_CHANNELS; }
    boolean isEmpty() { return channels.isEmpty(); }
}
