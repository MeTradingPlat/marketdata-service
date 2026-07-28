package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import com.metradingplat.marketdata.domain.enums.EnumOrderAction;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
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
     * Estado del probe de fundamentales en vivo (diagnostico temporal): una
     * conexion DxLink DEDICADA y separada de la principal, para que sus canales
     * nunca compitan con los que usan velas o el canal por defecto de Quote/Trade.
     * Se elimina despues de la prueba.
     */
    private DxLinkClient fundamentalsProbeConnection;
    private final List<DxLinkClient.DxLinkChannel> fundamentalsProbeChannels = new ArrayList<>();
    private final Set<String> fundamentalsProbeReceived = ConcurrentHashMap.newKeySet();
    private BiConsumer<String, com.metradingplat.marketdata.domain.models.FundamentalData> fundamentalsProbeListener;

    public Map<String, Object> startFundamentalsLiveProbe(List<String> symbols, int channelCount) {
        if (fundamentalsProbeConnection != null) {
            return Map.of("error", "Probe already running, stop it first");
        }
        String token = tastyTradeClient.getApiQuoteToken();
        String url = tastyTradeClient.getDxlinkUrl();
        fundamentalsProbeConnection = new DxLinkClient();
        fundamentalsProbeConnection.connect(url, token);
        if (!fundamentalsProbeConnection.isConnected()) {
            fundamentalsProbeConnection = null;
            return Map.of("error", "Dedicated probe connection failed to authenticate");
        }

        fundamentalsProbeListener = (symbol, data) -> fundamentalsProbeReceived.add(symbol);
        int perChannel = (int) Math.ceil(symbols.size() / (double) channelCount);
        for (int c = 0; c < channelCount; c++) {
            int start = c * perChannel;
            if (start >= symbols.size()) break;
            int end = Math.min(start + perChannel, symbols.size());
            try {
                DxLinkClient.DxLinkChannel ch = fundamentalsProbeConnection
                        .openNewChannel(Set.of("Summary", "Profile", "TradeETH"))
                        .get(10, TimeUnit.SECONDS);
                fundamentalsProbeChannels.add(ch);
                if (fundamentalsProbeChannels.size() == 1) ch.addFundamentalListener(fundamentalsProbeListener);
                ch.subscribeFundamentalsBatch(symbols.subList(start, end));
            } catch (Exception e) {
                log.warn("Fundamentals probe channel {}/{} failed: {}", c + 1, channelCount, e.getMessage());
            }
        }

        return Map.of(
                "channelsRequested", channelCount,
                "channelsOpened", fundamentalsProbeChannels.size(),
                "symbolsSubscribed", symbols.size());
    }

    public Map<String, Object> getFundamentalsLiveProbeStatus() {
        if (fundamentalsProbeConnection == null) {
            return Map.of("running", false);
        }
        return Map.of(
                "running", true,
                "connected", fundamentalsProbeConnection.isConnected(),
                "channelsOpen", fundamentalsProbeChannels.size(),
                "symbolsWithData", fundamentalsProbeReceived.size());
    }

    public Map<String, Object> stopFundamentalsLiveProbe() {
        if (fundamentalsProbeConnection == null) {
            return Map.of("wasRunning", false);
        }
        int finalCount = fundamentalsProbeReceived.size();
        if (!fundamentalsProbeChannels.isEmpty() && fundamentalsProbeListener != null) {
            fundamentalsProbeChannels.get(0).removeFundamentalListener(fundamentalsProbeListener);
        }
        for (var ch : fundamentalsProbeChannels) ch.close();
        fundamentalsProbeConnection.disconnect();
        fundamentalsProbeChannels.clear();
        fundamentalsProbeReceived.clear();
        fundamentalsProbeConnection = null;
        fundamentalsProbeListener = null;
        return Map.of("wasRunning", true, "finalSymbolsWithData", finalCount);
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
