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

    /**
     * Diagnostico temporal: confirma si el techo de ~8 canales concurrentes es
     * por conexion WebSocket o por cuenta/token TastyTrade. Ocupa los 8 canales
     * de la conexion principal y, mientras siguen abiertos, abre una SEGUNDA
     * conexion DxLink totalmente independiente (token/url frescos) e intenta
     * abrir sus propios canales. Se elimina despues de la prueba.
     */
    public Map<String, Object> testDualConnectionChannelLimit() {
        List<DxLinkClient.DxLinkChannel> mainChannels = new java.util.ArrayList<>();
        DxLinkClient second = null;
        List<DxLinkClient.DxLinkChannel> secondChannels = new java.util.ArrayList<>();
        try {
            for (int i = 0; i < 8; i++) {
                try {
                    mainChannels.add(dxLinkClient.openNewChannel(java.util.Set.of("Candle"))
                            .get(10, java.util.concurrent.TimeUnit.SECONDS));
                } catch (Exception e) {
                    log.warn("Main connection channel {}/8 failed: {}", i + 1, e.getMessage());
                    break;
                }
            }

            String token = tastyTradeClient.getApiQuoteToken();
            String url = tastyTradeClient.getDxlinkUrl();
            second = new DxLinkClient();
            second.connect(url, token);

            int secondOpened = 0;
            List<String> secondFailures = new java.util.ArrayList<>();
            if (second.isConnected()) {
                for (int i = 0; i < 10; i++) {
                    try {
                        secondChannels.add(second.openNewChannel(java.util.Set.of("Candle"))
                                .get(10, java.util.concurrent.TimeUnit.SECONDS));
                        secondOpened++;
                    } catch (Exception e) {
                        secondFailures.add("channel " + (i + 1) + ": " + e.getMessage());
                    }
                }
            }

            return Map.of(
                    "mainConnectionChannelsOpened", mainChannels.size(),
                    "secondConnectionConnected", second.isConnected(),
                    "secondConnectionChannelsAttempted", 10,
                    "secondConnectionChannelsOpened", secondOpened,
                    "secondConnectionFailures", secondFailures);
        } finally {
            for (var ch : secondChannels) ch.close();
            if (second != null) second.disconnect();
            for (var ch : mainChannels) ch.close();
        }
    }
}
