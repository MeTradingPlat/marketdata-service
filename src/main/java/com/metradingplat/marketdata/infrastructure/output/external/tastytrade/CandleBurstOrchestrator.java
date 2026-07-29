package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;
import jakarta.annotation.PreDestroy;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Semaphore;

/**
 * Reparte una peticion grande de velas historicas entre varias conexiones
 * DxLink efimeras en paralelo, cada una corriendo CandleWaveFetcher sobre su
 * propio shard, en vez de encadenar todas las oleadas en una sola conexion.
 * Nunca toca el DxLinkClient singleton principal -- cada shard abre y cierra
 * su propia conexion (token/URL propios), igual que se valido esta sesion.
 *
 * La cantidad de shards de UNA peticion no tiene techo propio -- se calcula
 * segun cuantas oleadas de 800 realmente hacen falta, sin importar el
 * tamano. Lo que si tiene techo es cuantas conexiones efimeras puede haber
 * ABIERTAS A LA VEZ EN TODO EL SERVICIO (el semaforo de abajo), para que
 * varias peticiones grandes simultaneas (de distintos escaneres) no sumen
 * cientos de conexiones de golpe contra TastyTrade -- se turnan en vez de
 * apilarse.
 */
@Slf4j
@Component
@RequiredArgsConstructor
public class CandleBurstOrchestrator {

    private final TastyTradeClient tastyTradeClient;
    private final TastyTradeConfig config;
    private final CandleWaveFetcher candleWaveFetcher;
    private final ExecutorService executor = Executors.newVirtualThreadPerTaskExecutor();
    private final Semaphore connectionBudget = new Semaphore(0);
    private volatile boolean budgetInitialized;

    public int getThresholdSymbols() {
        return config.getCandleBurst().getThresholdSymbols();
    }

    public int getAvailableConnectionPermits() {
        ensureBudgetInitialized();
        return connectionBudget.availablePermits();
    }

    private void ensureBudgetInitialized() {
        if (budgetInitialized) return;
        synchronized (this) {
            if (budgetInitialized) return;
            connectionBudget.release(config.getCandleBurst().getMaxConcurrentConnections());
            budgetInitialized = true;
        }
    }

    public Map<String, List<Candle>> fetchBurst(List<String> symbols, EnumTimeframe timeframe, Instant fromTime,
            String period, String type) {
        ensureBudgetInitialized();
        List<List<String>> shards = shard(symbols);
        log.info("Candle burst: {} symbols across {} connections", symbols.size(), shards.size());
        List<CompletableFuture<Map<String, List<Candle>>>> futures = shards.stream()
                .map(shardSymbols -> CompletableFuture.supplyAsync(
                        () -> fetchShardWithRetry(shardSymbols, timeframe, fromTime, period, type), executor))
                .toList();
        Map<String, List<Candle>> resultado = new ConcurrentHashMap<>();
        futures.forEach(f -> resultado.putAll(f.join()));
        return resultado;
    }

    private Map<String, List<Candle>> fetchShardWithRetry(List<String> shardSymbols, EnumTimeframe timeframe,
            Instant fromTime, String period, String type) {
        try {
            return fetchShard(shardSymbols, timeframe, fromTime, period, type);
        } catch (Exception e) {
            log.warn("Candle burst shard failed, retrying on a fresh connection: {}", e.getMessage());
            try {
                return fetchShard(shardSymbols, timeframe, fromTime, period, type);
            } catch (Exception retryFailure) {
                log.error("Candle burst shard failed again, returning partial results: {}", retryFailure.getMessage());
                return Map.of();
            }
        }
    }

    private Map<String, List<Candle>> fetchShard(List<String> shardSymbols, EnumTimeframe timeframe,
            Instant fromTime, String period, String type) throws InterruptedException {
        connectionBudget.acquire();
        try {
            DxLinkClient client = new DxLinkClient();
            try {
                client.connect(tastyTradeClient.getDxlinkUrl(), tastyTradeClient.getApiQuoteToken());
                if (!client.isConnected()) throw new IllegalStateException("Burst connection failed to authenticate");
                return candleWaveFetcher.fetchAllWaves(client, shardSymbols, timeframe, fromTime, period, type);
            } finally {
                client.disconnect();
            }
        } finally {
            connectionBudget.release();
        }
    }

    private List<List<String>> shard(List<String> symbols) {
        int shardCount = Math.max(1, (int) Math.ceil(symbols.size() / (double) CandleWaveFetcher.WAVE_MAX_SYMBOLS));
        int perShard = (int) Math.ceil(symbols.size() / (double) shardCount);
        List<List<String>> shards = new ArrayList<>();
        for (int i = 0; i < symbols.size(); i += perShard) {
            shards.add(symbols.subList(i, Math.min(i + perShard, symbols.size())));
        }
        return shards;
    }

    @PreDestroy
    public void shutdown() {
        executor.shutdown();
    }
}
