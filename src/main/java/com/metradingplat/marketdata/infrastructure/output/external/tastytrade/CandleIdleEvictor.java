package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.List;

/**
 * Limpieza real de inactividad del pool de velas: a diferencia del viejo
 * cleanupStaleCandles() (que solo olvidaba bookkeeping local sin avisarle a
 * DxLink), esta si desuscribe cada simbolo idle, cierra canales que quedan
 * vacios, y desconecta conexiones que se quedan sin canales -- liberando de
 * verdad la capacidad para que el pool pueda crecer en otro lado.
 */
@Slf4j
@Component
@RequiredArgsConstructor
class CandleIdleEvictor {

    private final CandleCacheStore cacheStore;
    private final CandleChannelAllocator allocator;

    void evict(List<PooledCandleConnection> connections) {
        long now = System.currentTimeMillis();
        for (PooledCandleConnection conn : connections) {
            for (PooledCandleChannel ch : conn.channels) {
                evictIdleSymbols(ch, now);
            }
        }
        closeEmptyChannels(connections);
        disconnectEmptyConnections(connections);
    }

    private void evictIdleSymbols(PooledCandleChannel ch, long now) {
        List<String> stale = new ArrayList<>();
        for (String key : ch.keys()) {
            if (now - ch.lastAccess(key) > maxIdleMillis(key)) stale.add(key);
        }
        for (String key : stale) {
            String[] parts = key.split("\\|");
            String label = EnumTimeframe.valueOf(parts[1]).getLabel();
            ch.channel.unsubscribeCandle(parts[0], label.substring(0, label.length() - 1), label.substring(label.length() - 1));
            ch.remove(key);
            cacheStore.remove(key);
            log.info("Candle auto-unsubscribe: {}", key);
        }
    }

    private void closeEmptyChannels(List<PooledCandleConnection> connections) {
        for (PooledCandleConnection conn : connections) {
            List<PooledCandleChannel> empties = conn.channels.stream().filter(PooledCandleChannel::isEmpty).toList();
            for (PooledCandleChannel ch : empties) {
                ch.channel.close();
                conn.channels.remove(ch);
                log.info("Candle pool channel closed (idle)");
            }
        }
    }

    private void disconnectEmptyConnections(List<PooledCandleConnection> connections) {
        List<PooledCandleConnection> empties = connections.stream().filter(PooledCandleConnection::isEmpty).toList();
        for (PooledCandleConnection conn : empties) {
            conn.client.disconnect();
            connections.remove(conn);
            allocator.releaseConnection();
            log.info("Candle pool connection closed (idle)");
        }
    }

    private static long maxIdleMillis(String key) {
        EnumTimeframe tf = EnumTimeframe.valueOf(key.split("\\|")[1]);
        return Math.max(tf.getDuration().multipliedBy(2).toMillis(), 300_000);
    }
}
