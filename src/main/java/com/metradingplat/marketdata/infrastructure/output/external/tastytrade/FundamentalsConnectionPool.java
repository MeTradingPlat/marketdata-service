package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import com.metradingplat.marketdata.domain.models.FundamentalData;
import jakarta.annotation.PreDestroy;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.function.BiConsumer;

/**
 * Mantiene N conexiones DxLink persistentes y dedicadas, cada una suscrita en
 * vivo (Summary/Profile/TradeETH via subscribeFundamentalsBatch) a un shard
 * del universo de fundamentales -- nunca el DxLinkClient singleton principal.
 * Cada conexion se reconecta sola (logica ya existente en DxLinkClient); este
 * pool solo orquesta el arranque y vuelve a suscribir el shard de cada
 * conexion despues de una reconexion via el hook onReconnected.
 */
@Slf4j
@Component
@RequiredArgsConstructor
public class FundamentalsConnectionPool {

    private final TastyTradeClient tastyTradeClient;
    private final TastyTradeConfig config;
    private final FundamentalsShardAssigner shardAssigner;
    private final ExecutorService executor = Executors.newVirtualThreadPerTaskExecutor();
    private final List<PooledConnection> pool = new CopyOnWriteArrayList<>();

    private record PooledConnection(DxLinkClient client, List<String> shard) {}

    public void start(List<String> universe, BiConsumer<String, FundamentalData> mergeCallback) {
        var poolConfig = config.getConnectionPool();
        List<List<String>> shards = shardAssigner.assign(universe, poolConfig.getConnectionCount(), poolConfig.getSymbolsPerConnection());
        log.info("Starting fundamentals connection pool: {} connections for {} symbols", shards.size(), universe.size());
        List<CompletableFuture<Void>> futures = shards.stream()
                .map(shard -> CompletableFuture.runAsync(() -> startConnection(shard, mergeCallback), executor))
                .toList();
        CompletableFuture.allOf(futures.toArray(new CompletableFuture[0])).join();
        log.info("Fundamentals connection pool started: {}/{} connections up", connectedCount(), pool.size());
    }

    private void startConnection(List<String> initialShard, BiConsumer<String, FundamentalData> mergeCallback) {
        List<String> shard = new CopyOnWriteArrayList<>(initialShard);
        DxLinkClient client = new DxLinkClient();
        client.setTokenRefresher(tastyTradeClient::getApiQuoteToken);
        client.connect(tastyTradeClient.getDxlinkUrl(), tastyTradeClient.getApiQuoteToken());
        client.setOnFundamentalData(mergeCallback);
        client.setOnReconnected(ch -> ch.subscribeFundamentalsBatch(new ArrayList<>(shard)));
        if (client.isConnected() && client.getDefaultChannel() != null) {
            client.getDefaultChannel().subscribeFundamentalsBatch(shard);
        } else {
            log.warn("Fundamentals pool connection failed to authenticate on startup -- will subscribe once its own reconnect logic succeeds");
        }
        pool.add(new PooledConnection(client, shard));
    }

    public boolean ensureSubscribed(String symbol) {
        for (PooledConnection pc : pool) {
            if (pc.shard().contains(symbol)) return true;
        }
        int capacity = config.getConnectionPool().getSymbolsPerConnection();
        for (PooledConnection pc : pool) {
            if (pc.shard().size() >= capacity || !pc.client().isConnected() || pc.client().getDefaultChannel() == null) continue;
            pc.client().getDefaultChannel().subscribeFundamentalsBatch(List.of(symbol));
            pc.shard().add(symbol);
            log.info("Added new symbol {} to fundamentals pool", symbol);
            return true;
        }
        log.warn("Could not subscribe new symbol {} to fundamentals pool -- all connections at capacity or down", symbol);
        return false;
    }

    public List<Boolean> connectionStatuses() {
        return pool.stream().map(pc -> pc.client().isConnected()).toList();
    }

    private long connectedCount() {
        return pool.stream().filter(pc -> pc.client().isConnected()).count();
    }

    @PreDestroy
    public void shutdown() {
        pool.forEach(pc -> pc.client().disconnect());
        executor.shutdown();
    }
}
