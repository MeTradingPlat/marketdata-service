package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import com.metradingplat.marketdata.domain.enums.EnumOrderAction;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.function.BiConsumer;

@Service
@RequiredArgsConstructor
@Slf4j
public class StressTestService {

    private final TastyTradeService tastyTradeService;
    private final TastyTradeClient tastyTradeClient;
    private final DxLinkClient dxLinkClient;

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

    /**
     * Diagnostico temporal: mide cuantos simbolos realmente entregan Summary/
     * Profile/TradeETH en un solo canal, antes de disenar el reparto multi-canal
     * para fundamentales en vivo. Usa una conexion DxLink separada y dedicada
     * para no competir con los canales de velas ni con el canal por defecto.
     * Se elimina despues de la prueba.
     */
    public Map<String, Object> testFundamentalsChannelCapacity(List<String> symbols, int waitSeconds) {
        DxLinkClient probe = null;
        DxLinkClient.DxLinkChannel channel = null;
        Set<String> received = ConcurrentHashMap.newKeySet();
        BiConsumer<String, com.metradingplat.marketdata.domain.models.FundamentalData> listener =
                (symbol, data) -> received.add(symbol);
        try {
            String token = tastyTradeClient.getApiQuoteToken();
            String url = tastyTradeClient.getDxlinkUrl();
            probe = new DxLinkClient();
            probe.connect(url, token);
            if (!probe.isConnected()) {
                return Map.of("connected", false, "symbolsRequested", symbols.size());
            }

            channel = probe.openNewChannel(Set.of("Summary", "Profile", "TradeETH"))
                    .get(10, TimeUnit.SECONDS);
            channel.addFundamentalListener(listener);
            channel.subscribeFundamentalsBatch(symbols);

            long deadline = System.currentTimeMillis() + waitSeconds * 1000L;
            while (System.currentTimeMillis() < deadline) {
                Thread.sleep(1000);
            }

            return Map.of(
                    "connected", true,
                    "symbolsRequested", symbols.size(),
                    "waitSeconds", waitSeconds,
                    "symbolsWithData", received.size(),
                    "sampleReceived", received.stream().limit(20).toList());
        } catch (Exception e) {
            log.error("Fundamentals channel capacity test failed: {}", e.getMessage());
            return Map.of("error", String.valueOf(e.getMessage()), "symbolsWithData", received.size());
        } finally {
            if (channel != null) {
                channel.removeFundamentalListener(listener);
                channel.close();
            }
            if (probe != null) probe.disconnect();
        }
    }
}
