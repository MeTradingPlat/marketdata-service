package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import java.time.Instant;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.ScheduledFuture;
import java.util.concurrent.Semaphore;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import java.util.concurrent.atomic.AtomicReference;

import org.springframework.stereotype.Service;

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

    /**
     * Máx. símbolos por mensaje FEED_SUBSCRIPTION → mensaje DxLink ≤ 60 KB.
     * Cada chunk va en su propio canal — DxLink solo procesa el primer FEED_SUBSCRIPTION
     * con fromTime por canal; mensajes adicionales reciben INVALID_MESSAGE.
     */
    private static final int CHUNK_SIZE = 200;

    /**
     * Timeout (segundos) por chunk cuando no llega ningún dato.
     * Con mercado cerrado la mayoría de chunks no devuelven nada; 8s evita
     * que un batch de 63 chunks tarde 63×30s=31min (ahora 63×8s≈8min máx).
     */
    private static final int CHUNK_NO_DATA_TIMEOUT_SECONDS = 8;

    /**
     * Garantiza que solo haya UN canal DxLink histórico abierto en cualquier momento.
     * DxLink envía INVALID_MESSAGE cuando se abren canales concurrentes, incluso pocos.
     * Además los canales cerrados localmente quedan como "fantasmas" en el servidor
     * (no soporta CHANNEL_CANCEL), acumulándose hasta disparar el límite.
     * El semáforo se adquiere POR CHUNK (no por batch entero), permitiendo que
     * los scanners se intercalen entre chunks de un batch grande.
     */
    private static final Semaphore BATCH_SEMAPHORE = new Semaphore(1);

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
     * Obtiene candles históricos de múltiples símbolos.
     * Cada chunk de 200 símbolos usa su propio canal DxLink con un solo FEED_SUBSCRIPTION,
     * porque DxLink solo procesa el primer FEED_SUBSCRIPTION con fromTime por canal.
     * El Semaphore(1) se adquiere/libera POR CHUNK, permitiendo que batches de scanners
     * se intercalen entre chunks de un batch grande (ej. 12k símbolos = 63 chunks).
     */
    public Map<String, List<Candle>> getCandlesBatch(List<String> symbols, EnumTimeframe timeframe, int bars) {
        log.info("Batch fetch: {} simbolos, timeframe={}, bars={}", symbols.size(), timeframe, bars);

        ensureConnected();

        Instant now = Instant.now();
        long fromTime = now.minus(timeframe.getDuration().multipliedBy(bars + 10)).toEpochMilli();
        String tf = timeframe.getLabel();

        List<List<String>> chunks = partition(symbols, CHUNK_SIZE);
        Map<String, List<Candle>> resultado = new HashMap<>();

        log.info("Batch: {} simbolos en {} chunks de max {} simbolos c/u",
                symbols.size(), chunks.size(), CHUNK_SIZE);

        for (int ci = 0; ci < chunks.size(); ci++) {
            List<String> chunk = chunks.get(ci);

            try {
                BATCH_SEMAPHORE.acquire();
            } catch (InterruptedException ie) {
                Thread.currentThread().interrupt();
                log.warn("Batch interrumpido esperando slot DxLink en chunk {}/{}", ci + 1, chunks.size());
                break;
            }

            try {
                Map<String, List<Candle>> chunkResult = processChunk(
                        chunk, tf, fromTime, timeframe, ci + 1, chunks.size());
                resultado.putAll(chunkResult);
            } finally {
                BATCH_SEMAPHORE.release();
            }
        }

        log.info("Batch completo: {}/{} simbolos con datos", resultado.size(), symbols.size());
        return resultado;
    }

    /**
     * Procesa un chunk de símbolos: abre canal dedicado, envía 1 FEED_SUBSCRIPTION,
     * espera datos con settle timer, cierra canal.
     */
    private Map<String, List<Candle>> processChunk(List<String> symbols, String tf, long fromTime,
            EnumTimeframe timeframe, int chunkNum, int totalChunks) {

        DxLinkClient.DxLinkChannel channel;
        try {
            channel = dxLinkClient.openNewChannel().get(10, TimeUnit.SECONDS);
        } catch (Exception e) {
            log.error("Chunk {}/{}: no se pudo abrir canal DxLink: {}", chunkNum, totalChunks, e.getMessage());
            return Map.of();
        }

        Set<String> symbolSet = new HashSet<>(symbols);
        ConcurrentHashMap<String, List<Candle>> candlesPorSimbolo = new ConcurrentHashMap<>();
        Set<String> simbolosConDatos = ConcurrentHashMap.newKeySet();
        CompletableFuture<Void> completed = new CompletableFuture<>();
        AtomicReference<ScheduledFuture<?>> settleTask = new AtomicReference<>();
        ScheduledExecutorService scheduler = Executors.newSingleThreadScheduledExecutor();

        DxLinkClient.CandleCallback listener = (sym, candle, isComplete) -> {
            if (!symbolSet.contains(sym)) return;

            candle.setTimeframe(timeframe);
            candlesPorSimbolo.computeIfAbsent(sym, k -> new ArrayList<>());
            synchronized (candlesPorSimbolo.get(sym)) {
                candlesPorSimbolo.get(sym).add(candle);
            }
            simbolosConDatos.add(sym);

            if (isComplete) {
                if (scheduler.isShutdown()) return;
                boolean allResponded = simbolosConDatos.size() >= symbols.size();
                try {
                    ScheduledFuture<?> prev = settleTask.getAndSet(
                        scheduler.schedule(() -> completed.complete(null),
                            allResponded ? 300 : 2000, TimeUnit.MILLISECONDS)
                    );
                    if (prev != null) prev.cancel(false);
                } catch (java.util.concurrent.RejectedExecutionException ignored) {
                }
            } else {
                ScheduledFuture<?> pending = settleTask.get();
                if (pending != null) pending.cancel(false);
            }
        };

        channel.addCandleListener(listener);

        // 1 FEED_SUBSCRIPTION por canal — la clave del fix
        List<Map<String, Object>> items = symbols.stream()
                .map(s -> Map.<String, Object>of(
                        "symbol", s + "{=" + tf + "}",
                        "type", "Candle",
                        "fromTime", fromTime))
                .toList();
        channel.subscribeCandlesBatch(items);
        log.info("Chunk {}/{}: canal {} suscrito a {} simbolos",
                chunkNum, totalChunks, channel.getId(), symbols.size());

        try {
            completed.get(CHUNK_NO_DATA_TIMEOUT_SECONDS, TimeUnit.SECONDS);
            log.debug("Chunk {}/{}: canal {} completo — {}/{} simbolos con datos",
                    chunkNum, totalChunks, channel.getId(), simbolosConDatos.size(), symbols.size());
        } catch (TimeoutException e) {
            int received = simbolosConDatos.size();
            if (received == 0) {
                log.debug("Chunk {}/{}: timeout {}s sin datos (mercado cerrado?) — {} simbolos",
                        chunkNum, totalChunks, CHUNK_NO_DATA_TIMEOUT_SECONDS, symbols.size());
            } else {
                log.info("Chunk {}/{}: timeout {}s — parcial {}/{} simbolos",
                        chunkNum, totalChunks, CHUNK_NO_DATA_TIMEOUT_SECONDS, received, symbols.size());
            }
        } catch (Exception e) {
            log.error("Chunk {}/{} error", chunkNum, totalChunks, e);
        }

        scheduler.shutdownNow();
        channel.removeCandleListener(listener);
        channel.close();

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
    }

    /** Divide una lista en sublistas de tamaño maximo {@code size}. */
    private static <T> List<List<T>> partition(List<T> list, int size) {
        List<List<T>> result = new ArrayList<>();
        for (int i = 0; i < list.size(); i += size) {
            result.add(list.subList(i, Math.min(i + size, list.size())));
        }
        return result;
    }

    /**
     * Obtiene candles actuales sin cache (Bypass Db)
     */
    public Map<String, List<Candle>> getCandlesBatchNoCache(List<String> symbols, EnumTimeframe timeframe, int bars) {
        return getCandlesBatch(symbols, timeframe, bars);
    }

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
