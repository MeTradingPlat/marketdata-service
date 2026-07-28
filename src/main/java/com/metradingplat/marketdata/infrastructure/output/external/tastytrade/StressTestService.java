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
    private final List<DxLinkClient> fundamentalsProbeConnections = new ArrayList<>();
    private final List<DxLinkClient.DxLinkChannel> fundamentalsProbeChannels = new ArrayList<>();
    private final Set<String> fundamentalsProbeReceived = ConcurrentHashMap.newKeySet();
    private final List<BiConsumer<String, com.metradingplat.marketdata.domain.models.FundamentalData>> fundamentalsProbeListeners = new ArrayList<>();

    /**
     * A diferencia de repartir en varios canales de UNA sola conexion (probado
     * y estancado en ~3325/13130 sin importar cuanto se esperara), esto abre
     * una conexion DxLink SEPARADA por cada porcion de simbolos -- misma logica
     * que confirmo que el techo de 8 canales es por conexion, aplicada ahora a
     * suscripciones en vivo en vez de canales nuevos.
     */
    public Map<String, Object> startFundamentalsLiveProbeMultiConnection(List<String> symbols, int connectionCount) {
        if (!fundamentalsProbeConnections.isEmpty()) {
            return Map.of("error", "Probe already running, stop it first");
        }
        int perConnection = (int) Math.ceil(symbols.size() / (double) connectionCount);
        int opened = 0;
        for (int c = 0; c < connectionCount; c++) {
            int start = c * perConnection;
            if (start >= symbols.size()) break;
            int end = Math.min(start + perConnection, symbols.size());
            try {
                String token = tastyTradeClient.getApiQuoteToken();
                String url = tastyTradeClient.getDxlinkUrl();
                DxLinkClient conn = new DxLinkClient();
                conn.connect(url, token);
                if (!conn.isConnected()) {
                    log.warn("Fundamentals probe connection {}/{} failed to authenticate", c + 1, connectionCount);
                    continue;
                }
                DxLinkClient.DxLinkChannel ch = conn.openNewChannel(Set.of("Summary", "Profile", "TradeETH"))
                        .get(10, TimeUnit.SECONDS);
                BiConsumer<String, com.metradingplat.marketdata.domain.models.FundamentalData> listener =
                        (symbol, data) -> fundamentalsProbeReceived.add(symbol);
                ch.addFundamentalListener(listener);
                ch.subscribeFundamentalsBatch(symbols.subList(start, end));
                fundamentalsProbeConnections.add(conn);
                fundamentalsProbeChannels.add(ch);
                fundamentalsProbeListeners.add(listener);
                opened++;
            } catch (Exception e) {
                log.warn("Fundamentals probe connection {}/{} failed: {}", c + 1, connectionCount, e.getMessage());
            }
        }

        return Map.of(
                "connectionsRequested", connectionCount,
                "connectionsOpened", opened,
                "symbolsSubscribed", symbols.size());
    }

    public Map<String, Object> getFundamentalsLiveProbeStatus() {
        if (fundamentalsProbeConnections.isEmpty()) {
            return Map.of("running", false);
        }
        return Map.of(
                "running", true,
                "connectionsOpen", fundamentalsProbeConnections.size(),
                "symbolsWithData", fundamentalsProbeReceived.size());
    }

    public Map<String, Object> stopFundamentalsLiveProbe() {
        if (fundamentalsProbeConnections.isEmpty()) {
            return Map.of("wasRunning", false);
        }
        int finalCount = fundamentalsProbeReceived.size();
        for (int i = 0; i < fundamentalsProbeChannels.size(); i++) {
            fundamentalsProbeChannels.get(i).removeFundamentalListener(fundamentalsProbeListeners.get(i));
            fundamentalsProbeChannels.get(i).close();
        }
        for (var conn : fundamentalsProbeConnections) conn.disconnect();
        fundamentalsProbeConnections.clear();
        fundamentalsProbeChannels.clear();
        fundamentalsProbeListeners.clear();
        fundamentalsProbeReceived.clear();
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
