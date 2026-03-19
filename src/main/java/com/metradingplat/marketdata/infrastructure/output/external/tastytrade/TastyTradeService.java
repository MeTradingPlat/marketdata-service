package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import java.time.Instant;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.ScheduledFuture;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import java.util.concurrent.atomic.AtomicReference;

import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;
import org.springframework.web.server.ResponseStatusException;

import com.metradingplat.marketdata.application.output.GestionarChangeNotificationsProducerIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.ActiveEquity;
import com.metradingplat.marketdata.domain.models.BracketOrder;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.domain.models.OrderRequest;
import com.metradingplat.marketdata.domain.models.OrderResponse;

import jakarta.annotation.PostConstruct;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

/**
 * Servicio interno de TastyTrade.
 * Orquesta TastyTradeClient (REST) y DxLinkClient (WebSocket).
 * Los datos históricos se obtienen directamente de DxLink sin caché en BD.
 */
@Service
@RequiredArgsConstructor
@Slf4j
public class TastyTradeService {

    /** Máx. símbolos por chunk → mensaje DxLink ≤ 60 KB. */
    private static final int CHUNK_SIZE = 200;

    /**
     * Executor de virtual threads (Java 21 Project Loom).
     * Cada chunk abre su propio virtual thread — I/O-bound, sin overhead de OS threads.
     * Soporta cientos de chunks concurrentes sin pool fijo.
     */
    private static final ExecutorService BATCH_POOL = Executors.newVirtualThreadPerTaskExecutor();

    private final TastyTradeClient tastyTradeClient;
    private final DxLinkClient dxLinkClient;
    private final GestionarChangeNotificationsProducerIntPort kafkaProducer;

    @PostConstruct
    public void init() {
        log.info("Initializing TastyTrade service");

        // Configurar token refresher para auto-reconexión (before connect, does not need defaultChannel)
        dxLinkClient.setTokenRefresher(() -> {
            log.info("Token refresher called - obtaining fresh API quote token");
            return tastyTradeClient.getApiQuoteToken();
        });

        // Conectar a DxLink (this creates defaultChannel)
        try {
            log.debug("Obtaining API quote token from TastyTrade...");
            String token = tastyTradeClient.getApiQuoteToken();
            String url = tastyTradeClient.getDxlinkUrl();
            log.debug("Got token and URL. Connecting to DxLink at: {}", url);
            dxLinkClient.connect(url, token);
        } catch (Exception e) {
            log.error("Failed to initialize TastyTrade service: {}", e.getMessage(), e);
        }

        // Configurar callbacks AFTER connect() so defaultChannel exists
        dxLinkClient.setOnMarketData((symbol, data) -> {
            log.debug("Market data received for {}: bid={}, ask={}, last={}",
                    symbol, data.getBid(), data.getAsk(), data.getLastPrice());
            kafkaProducer.publishMarketData(data);
        });

        dxLinkClient.setOnCandle((symbol, candle, isComplete) -> {
            log.debug("Candle received for {}: {} O={} H={} L={} C={} complete={}",
                    symbol, candle.getTimestamp(), candle.getOpen(),
                    candle.getHigh(), candle.getLow(), candle.getClose(), isComplete);
        });
    }

    public void sendOrder(OrderRequest request) {
        log.info("Sending order: {} {} {} @ {}",
                request.getAction(), request.getQuantity(), request.getSymbol(), request.getPrice());
        tastyTradeClient.submitOrder(request);
    }

    public void subscribe(String symbol) {
        log.info("Subscribing to real-time data: {}", symbol);
        ensureConnected();
        dxLinkClient.subscribe(symbol);
    }

    public void unsubscribe(String symbol) {
        log.info("Unsubscribing from: {}", symbol);
        dxLinkClient.unsubscribe(symbol);
    }

    /**
     * Obtiene candles historicos de un solo simbolo. Delega al metodo batch.
     */
    public List<Candle> getCandles(String symbol, EnumTimeframe timeframe) {
        log.debug("Fetching candles for {} {}", symbol, timeframe);
        Map<String, List<Candle>> result = getCandlesBatch(List.of(symbol), timeframe, 700);
        return result.getOrDefault(symbol, List.of());
    }

    /**
     * Obtiene candles historicos de multiples simbolos.
     * Divide en chunks de CHUNK_SIZE y abre un canal DxLink por chunk en paralelo.
     * Soporta cualquier cantidad de simbolos (ej. 12 000+) sin timeout.
     */
    public Map<String, List<Candle>> getCandlesBatch(List<String> symbols, EnumTimeframe timeframe, int bars) {
        log.info("Batch fetch: {} simbolos, timeframe={}, bars={}", symbols.size(), timeframe, bars);

        Instant now = Instant.now();
        long fromTime = now.minus(timeframe.getDuration().multipliedBy(bars + 10)).toEpochMilli();

        Map<String, Long> symbolToFromTime = new HashMap<>();
        symbols.forEach(s -> symbolToFromTime.put(s, fromTime));

        List<List<String>> chunks = partition(symbols, CHUNK_SIZE);
        log.info("Dividiendo {} simbolos en {} chunks (max {} por canal)", symbols.size(), chunks.size(), CHUNK_SIZE);

        List<CompletableFuture<Map<String, List<Candle>>>> futures = chunks.stream()
                .map(chunk -> CompletableFuture.supplyAsync(
                        () -> fetchCandlesBatchFromDxLink(chunk, timeframe, symbolToFromTime),
                        BATCH_POOL))
                .toList();

        Map<String, List<Candle>> resultado = new HashMap<>();
        try {
            CompletableFuture.allOf(futures.toArray(new CompletableFuture[0])).get(90, TimeUnit.SECONDS);
        } catch (TimeoutException e) {
            log.warn("Parallel batch timeout 90s — retornando resultados parciales de chunks completados");
        } catch (Exception e) {
            log.error("Parallel batch error: {}", e.getMessage(), e);
        }

        for (CompletableFuture<Map<String, List<Candle>>> f : futures) {
            if (f.isDone() && !f.isCompletedExceptionally()) {
                try {
                    resultado.putAll(f.get());
                } catch (Exception e) {
                    log.warn("Error obteniendo resultado de chunk: {}", e.getMessage());
                }
            }
        }

        log.info("Parallel batch completo: {}/{} simbolos con datos", resultado.size(), symbols.size());
        return resultado;
    }

    /** Divide una lista en sublistas de tamaño maximo {@code size}. */
    private static <T> List<List<T>> partition(List<T> list, int size) {
        List<List<T>> result = new ArrayList<>();
        for (int i = 0; i < list.size(); i += size) {
            result.add(list.subList(i, Math.min(i + size, list.size())));
        }
        return result;
    }

    private Map<String, List<Candle>> fetchCandlesBatchFromDxLink(
            List<String> symbols, EnumTimeframe timeframe, Map<String, Long> symbolToFromTime) {

        ensureConnected();

        // Abre un canal dedicado para este chunk (no el default) → canales paralelos sin interferencia
        DxLinkClient.DxLinkChannel channel;
        try {
            channel = dxLinkClient.openNewChannel().get(10, TimeUnit.SECONDS);
        } catch (Exception e) {
            log.error("No se pudo abrir canal DxLink para batch: {}", e.getMessage());
            throw new ResponseStatusException(HttpStatus.SERVICE_UNAVAILABLE, "DxLink channel unavailable");
        }

        DxLinkClient.CandleCallback batchListener = null;
        ScheduledExecutorService scheduler = Executors.newSingleThreadScheduledExecutor();
        try {
            String tf = timeframe.getLabel();
            ConcurrentHashMap<String, List<Candle>> candlesPorSimbolo = new ConcurrentHashMap<>();
            Set<String> simbolosConDatos = ConcurrentHashMap.newKeySet();
            CompletableFuture<Void> allCompleted = new CompletableFuture<>();
            AtomicReference<ScheduledFuture<?>> settleTask = new AtomicReference<>();

            // Track when the first data arrives so we can start a settle timer
            // even if not all symbols respond (some may be delisted/invalid)
            AtomicReference<Instant> firstDataTime = new AtomicReference<>();

            batchListener = (sym, candle, isComplete) -> {
                if (!symbols.contains(sym))
                    return;

                candle.setTimeframe(timeframe);
                candlesPorSimbolo.computeIfAbsent(sym, k -> new ArrayList<>());
                synchronized (candlesPorSimbolo.get(sym)) {
                    candlesPorSimbolo.get(sym).add(candle);
                }
                simbolosConDatos.add(sym);
                firstDataTime.compareAndSet(null, Instant.now());

                if (isComplete) {
                    // TX_PENDING=0: start settle timer when all symbols responded,
                    // OR when we've been receiving data for a while (some symbols may never respond)
                    if (scheduler.isShutdown()) return; // batch ya completó, ignorar evento tardío
                    boolean allResponded = simbolosConDatos.containsAll(symbols);
                    try {
                        if (allResponded) {
                            ScheduledFuture<?> prev = settleTask.getAndSet(
                                scheduler.schedule(() -> allCompleted.complete(null), 300, TimeUnit.MILLISECONDS)
                            );
                            if (prev != null) prev.cancel(false);
                        } else {
                            // Even if not all symbols responded, start a longer settle timer
                            // so we don't wait forever for symbols that will never respond
                            ScheduledFuture<?> prev = settleTask.getAndSet(
                                scheduler.schedule(() -> allCompleted.complete(null), 2000, TimeUnit.MILLISECONDS)
                            );
                            if (prev != null) prev.cancel(false);
                        }
                    } catch (java.util.concurrent.RejectedExecutionException ignored) {
                        // scheduler cerrado entre el isShutdown() check y el schedule() — batch ya completó
                    }
                } else {
                    // TX_PENDING=1: más candles vienen → cancelar timer pendiente
                    ScheduledFuture<?> pending = settleTask.get();
                    if (pending != null) pending.cancel(false);
                }
            };

            channel.addCandleListener(batchListener);

            List<Map<String, Object>> subscriptionItems = symbols.stream()
                    .map(symbol -> {
                        String candleSymbol = symbol + "{=" + tf + "}";
                        return Map.<String, Object>of(
                                "symbol", candleSymbol,
                                "type", "Candle",
                                "fromTime", symbolToFromTime.get(symbol));
                    })
                    .toList();

            log.debug("Batch subscribing {} symbols on shared channel {}", symbols.size(), channel.getId());
            channel.subscribeCandlesBatch(subscriptionItems);

            int maxWaitSeconds = Math.min(15 + symbols.size() / 10, 60);

            try {
                allCompleted.get(maxWaitSeconds, TimeUnit.SECONDS);
                log.debug("Batch complete on shared channel {} successfully", channel.getId());
            } catch (TimeoutException e) {
                int received = simbolosConDatos.size();
                if (received == 0) {
                    log.warn("Batch timed out after {}s with ZERO data on channel {}. {} symbols sent, none responded.",
                            maxWaitSeconds, channel.getId(), symbols.size());
                } else {
                    log.warn("Batch timed out after {}s on channel {}. Got {}/{} symbols — returning partial results.",
                            maxWaitSeconds, channel.getId(), received, symbols.size());
                }
                // Return partial results instead of 503 — missing symbols get empty list
            } catch (Exception e) {
                log.error("Batch interrupted", e);
                throw new ResponseStatusException(HttpStatus.SERVICE_UNAVAILABLE, "DxLink batch failed", e);
            }

            Map<String, List<Candle>> resultado = new HashMap<>();
            for (Map.Entry<String, List<Candle>> entry : candlesPorSimbolo.entrySet()) {
                List<Candle> sorted;
                synchronized (entry.getValue()) {
                    sorted = entry.getValue().stream()
                            .sorted(Comparator.comparing(Candle::getTimestamp))
                            .toList();
                }
                resultado.put(entry.getKey(), sorted);
            }

            return resultado;

        } finally {
            scheduler.shutdownNow();
            if (batchListener != null) {
                channel.removeCandleListener(batchListener);
            }
            // Cierra el canal dedicado — lo elimina del mapa interno, libera recursos
            channel.close();
        }
    }

    /**
     * Obtiene candles actuales sin cache (Bypass Db)
     */
    public Map<String, List<Candle>> getCandlesBatchNoCache(List<String> symbols, EnumTimeframe timeframe, int bars) {
        return getCandlesBatch(symbols, timeframe, bars);
    }

    // CandleKey record removed

    public List<ActiveEquity> getActiveEquities(int pageOffset, int perPage) {
        return tastyTradeClient.getActiveEquities(pageOffset, perPage);
    }

    public Map<String, Object> getMarketDataByType(String symbol) {
        return tastyTradeClient.getMarketDataByType(symbol);
    }

    public List<Map<String, Object>> getEarningsReports(String symbol, String startDate) {
        return tastyTradeClient.getEarningsReports(symbol, startDate);
    }

    public OrderResponse sendBracketOrder(BracketOrder order) {
        return tastyTradeClient.submitBracketOrder(order);
    }

    public void cancelOrder(String orderId) {
        tastyTradeClient.cancelOrder(orderId);
    }

    private void ensureConnected() {
        if (!dxLinkClient.isConnected()) {
            log.debug("Reconnecting to DxLink");
            String token = tastyTradeClient.getApiQuoteToken();
            String url = tastyTradeClient.getDxlinkUrl();
            dxLinkClient.connect(url, token);
        }
    }
}
