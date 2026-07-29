package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import com.metradingplat.marketdata.domain.enums.EnumOrderAction;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.atomic.AtomicInteger;

@Service
@RequiredArgsConstructor
@Slf4j
public class StressTestService {

    private final TastyTradeService tastyTradeService;
    private final TastyTradeClient tastyTradeClient;
    private final DxLinkClient dxLinkClient;
    private final EquitiesUniverseProvider equitiesUniverseProvider;
    private final CandleChannelOpener candleChannelOpener;

    // DIAGNOSTICO TEMPORAL -- mide cuantos simbolos puede sostener una
    // suscripcion PERMANENTE de velas en vivo (no rafaga de historico) antes
    // de que el throughput se degrade, para decidir si vale la pena una
    // suscripcion continua a todo el universo como la que ya existe para
    // fundamentales. Se elimina despues de la prueba.
    private final List<DxLinkClient> candleProbeConnections = new CopyOnWriteArrayList<>();
    private final Map<String, Long> candleProbeLastEventAt = new ConcurrentHashMap<>();
    private volatile Instant candleProbeStartedAt;
    private volatile int candleProbeTargetSymbols;
    private final ExecutorService candleProbeExecutor = Executors.newVirtualThreadPerTaskExecutor();

    public synchronized Map<String, Object> startCandleLiveProbe(int connectionCount) {
        if (candleProbeStartedAt != null) {
            return Map.of("error", "already running, stop first");
        }
        List<String> universe = equitiesUniverseProvider.getUniverse();
        candleProbeTargetSymbols = universe.size();
        int shardSize = (int) Math.ceil(universe.size() / (double) connectionCount);
        List<CompletableFuture<Void>> futures = new ArrayList<>();
        for (int i = 0; i < connectionCount; i++) {
            int start = i * shardSize;
            if (start >= universe.size()) break;
            List<String> shard = universe.subList(start, Math.min(start + shardSize, universe.size()));
            futures.add(CompletableFuture.runAsync(() -> startCandleProbeConnection(shard), candleProbeExecutor));
        }
        candleProbeStartedAt = Instant.now();
        CompletableFuture.allOf(futures.toArray(new CompletableFuture[0])).join();
        return Map.of("status", "started", "connections", candleProbeConnections.size(), "targetSymbols", universe.size());
    }

    private void startCandleProbeConnection(List<String> shard) {
        DxLinkClient client = new DxLinkClient();
        client.connect(tastyTradeClient.getDxlinkUrl(), tastyTradeClient.getApiQuoteToken());
        if (!client.isConnected()) {
            log.warn("Candle probe connection failed to authenticate");
            return;
        }
        candleProbeConnections.add(client);
        try {
            List<DxLinkClient.DxLinkChannel> channels = candleChannelOpener.open(client, 8);
            if (channels.isEmpty()) return;
            channels.get(0).addCandleListener((symbol, candle, complete) ->
                    candleProbeLastEventAt.put(symbol, System.currentTimeMillis()));
            int perChannel = (int) Math.ceil(shard.size() / (double) channels.size());
            long fromTime = Instant.now().minus(Duration.ofMinutes(5)).toEpochMilli();
            for (int c = 0; c < channels.size(); c++) {
                int start = c * perChannel;
                if (start >= shard.size()) break;
                List<String> group = shard.subList(start, Math.min(start + perChannel, shard.size()));
                for (int i = 0; i < group.size(); i += 10) {
                    int end = Math.min(i + 10, group.size());
                    List<Map<String, Object>> items = new ArrayList<>();
                    for (String s : group.subList(i, end)) {
                        items.add(Map.of("symbol", String.format("%s{=1m}", s), "type", "Candle"));
                    }
                    channels.get(c).subscribeCandlesHistory(items, fromTime);
                }
            }
        } catch (Exception e) {
            log.error("Candle probe connection setup failed", e);
        }
    }

    public Map<String, Object> getCandleLiveProbeStatus() {
        if (candleProbeStartedAt == null) {
            return Map.of("status", "not running");
        }
        long now = System.currentTimeMillis();
        long recentCutoffMs = now - 60_000;
        long everCount = candleProbeLastEventAt.size();
        long recentCount = candleProbeLastEventAt.values().stream().filter(t -> t > recentCutoffMs).count();
        return Map.of(
                "elapsedSeconds", Duration.between(candleProbeStartedAt, Instant.now()).getSeconds(),
                "connections", candleProbeConnections.size(),
                "targetSymbols", candleProbeTargetSymbols,
                "symbolsWithAtLeastOneEvent", everCount,
                "symbolsWithEventInLast60s", recentCount);
    }

    public synchronized Map<String, Object> stopCandleLiveProbe() {
        for (DxLinkClient c : candleProbeConnections) c.disconnect();
        int count = candleProbeConnections.size();
        candleProbeConnections.clear();
        candleProbeLastEventAt.clear();
        candleProbeStartedAt = null;
        candleProbeTargetSymbols = 0;
        return Map.of("status", "stopped", "connectionsClosed", count);
    }

    /**
     * Suscribe masivamente a una lista de símbolos para probar el throughput del WebSocket.
     */
    public Map<String, Object> subscribeMassive(List<String> symbols) {
        log.info("🔥 STRESS TEST: Suscripción masiva a {} símbolos", symbols.size());
        
        long start = System.currentTimeMillis();
        for (String symbol : symbols) {
            tastyTradeService.subscribe(symbol);
        }
        long end = System.currentTimeMillis();

        return Map.of(
            "test", "Massive Subscription",
            "symbolsCount", symbols.size(),
            "executionTimeMs", (end - start),
            "status", "Subscription burst sent"
        );
    }

    /**
     * Ejecuta una ráfaga de validaciones Dry-Run (sin riesgo de capital) para probar concurrencia del Control Plane.
     */
    public CompletableFuture<Map<String, Object>> validateOrdersBurst(int count) {
        log.info("🔥 STRESS TEST: Ráfaga de {} validaciones Dry-Run", count);
        
        AtomicInteger success = new AtomicInteger(0);
        AtomicInteger failure = new AtomicInteger(0);
        long start = System.currentTimeMillis();

        List<CompletableFuture<Void>> futures = new java.util.ArrayList<>();
        
        // Simular una orden de prueba (AAPL Bracket)
        com.metradingplat.marketdata.domain.models.BracketOrder dummyOrder = 
            com.metradingplat.marketdata.domain.models.BracketOrder.builder()
                .symbol("AAPL")
                .action(EnumOrderAction.BUY_TO_OPEN)
                .quantity(1)
                .entryPrice(java.math.BigDecimal.valueOf(150.0))
                .takeProfitPrice(java.math.BigDecimal.valueOf(160.0))
                .stopLossPrice(java.math.BigDecimal.valueOf(140.0))
                .build();

        for (int i = 0; i < count; i++) {
            futures.add(CompletableFuture.runAsync(() -> {
                try {
                    tastyTradeClient.validateBracketOrder(dummyOrder);
                    success.incrementAndGet();
                } catch (Exception e) {
                    failure.incrementAndGet();
                }
            }));
        }

        return CompletableFuture.allOf(futures.toArray(new CompletableFuture[0]))
            .thenApply(v -> {
                long end = System.currentTimeMillis();
                return Map.of(
                    "test", "Dry-Run Burst",
                    "totalRequests", count,
                    "success", success.get(),
                    "failures", failure.get(),
                    "totalTimeMs", (end - start),
                    "avgLatencyMs", (double)(end - start) / count
                );
            });
    }

    public Map<String, Object> getSystemStats() {
        return dxLinkClient.getConnectionStats();
    }
}
