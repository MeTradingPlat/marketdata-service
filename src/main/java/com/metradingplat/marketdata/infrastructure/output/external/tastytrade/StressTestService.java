package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import com.metradingplat.marketdata.domain.enums.EnumOrderAction;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

import java.util.List;
import java.util.Map;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.atomic.AtomicInteger;

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
}
