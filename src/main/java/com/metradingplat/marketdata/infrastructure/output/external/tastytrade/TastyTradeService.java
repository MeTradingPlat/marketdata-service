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
     * Todos los mensajes van al MISMO canal — evita INVALID_MESSAGE por múltiples canales.
     */
    private static final int CHUNK_SIZE = 200;

    /**
     * Garantiza que solo haya UN canal DxLink histórico abierto en cualquier momento.
     * DxLink envía INVALID_MESSAGE cuando se abren canales concurrentes, incluso pocos.
     * Además los canales cerrados localmente quedan como "fantasmas" en el servidor
     * (no soporta CHANNEL_CANCEL), acumulándose hasta disparar el límite.
     * Con 1 permiso los batches se encolan: cada scanner tarda ~2-4s con mercado abierto,
     * 6 scanners × 3s = ~18s por ciclo — bien dentro del intervalo de 60s.
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
     * Obtiene candles historicos de multiples simbolos usando UN solo canal DxLink.
     * Envía N mensajes FEED_SUBSCRIPTION de 200 símbolos c/u al mismo canal,
     * evitando el INVALID_MESSAGE que ocurre al abrir múltiples canales en paralelo.
     * Soporta cualquier cantidad de símbolos (ej. 12 000+).
     */
    public Map<String, List<Candle>> getCandlesBatch(List<String> symbols, EnumTimeframe timeframe, int bars) {
        log.info("Batch fetch: {} simbolos, timeframe={}, bars={}", symbols.size(), timeframe, bars);

        ensureConnected();

        try {
            BATCH_SEMAPHORE.acquire();
        } catch (InterruptedException ie) {
            Thread.currentThread().interrupt();
            log.warn("Batch interrumpido esperando slot DxLink");
            return Map.of();
        }

        Instant now = Instant.now();
        long fromTime = now.minus(timeframe.getDuration().multipliedBy(bars + 10)).toEpochMilli();
        String tf = timeframe.getLabel();

        // Un solo canal para todos los símbolos
        DxLinkClient.DxLinkChannel channel;
        try {
            channel = dxLinkClient.openNewChannel().get(10, TimeUnit.SECONDS);
        } catch (Exception e) {
            BATCH_SEMAPHORE.release();
            log.error("No se pudo abrir canal DxLink para batch: {}", e.getMessage());
            return Map.of();
        }

        Set<String> symbolSet = new HashSet<>(symbols);
        ConcurrentHashMap<String, List<Candle>> candlesPorSimbolo = new ConcurrentHashMap<>();
        Set<String> simbolosConDatos = ConcurrentHashMap.newKeySet();
        CompletableFuture<Void> allCompleted = new CompletableFuture<>();
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
                        scheduler.schedule(() -> allCompleted.complete(null),
                            allResponded ? 300 : 2000, TimeUnit.MILLISECONDS)
                    );
                    if (prev != null) prev.cancel(false);
                } catch (java.util.concurrent.RejectedExecutionException ignored) {
                    // scheduler cerrado entre el isShutdown() check y el schedule()
                }
            } else {
                // TX_PENDING=1: más candles vienen → cancelar timer pendiente
                ScheduledFuture<?> pending = settleTask.get();
                if (pending != null) pending.cancel(false);
            }
        };

        channel.addCandleListener(listener);

        // Enviar N × FEED_SUBSCRIPTION al mismo canal (200 símbolos c/u).
        // DxLink rechaza (INVALID_MESSAGE) mensajes enviados en ráfaga rápida,
        // así que espaciamos 100ms entre cada mensaje cuando hay más de 1 chunk.
        List<List<String>> chunks = partition(symbols, CHUNK_SIZE);
        log.info("Canal {}: suscribiendo {} simbolos en {} mensajes FEED_SUBSCRIPTION",
                channel.getId(), symbols.size(), chunks.size());
        for (int ci = 0; ci < chunks.size(); ci++) {
            List<Map<String, Object>> items = chunks.get(ci).stream()
                    .map(s -> Map.<String, Object>of(
                            "symbol", s + "{=" + tf + "}",
                            "type", "Candle",
                            "fromTime", fromTime))
                    .toList();
            channel.subscribeCandlesBatch(items);
            // Espaciar mensajes para no disparar rate-limit de DxLink
            if (ci < chunks.size() - 1) {
                try { Thread.sleep(100); } catch (InterruptedException ie) {
                    Thread.currentThread().interrupt();
                    break;
                }
            }
        }

        // Timeout: 30s base + 1s por cada 100 símbolos, máx 120s
        int maxWaitSeconds = Math.min(30 + symbols.size() / 100, 120);
        try {
            allCompleted.get(maxWaitSeconds, TimeUnit.SECONDS);
            log.debug("Batch completo en canal {} con {}/{} simbolos",
                    channel.getId(), simbolosConDatos.size(), symbols.size());
        } catch (TimeoutException e) {
            int received = simbolosConDatos.size();
            if (received == 0) {
                log.warn("Batch timeout {}s ZERO data en canal {}. {} simbolos enviados.",
                        maxWaitSeconds, channel.getId(), symbols.size());
            } else {
                log.warn("Batch timeout {}s en canal {}. Recibidos {}/{} — resultados parciales.",
                        maxWaitSeconds, channel.getId(), received, symbols.size());
            }
        } catch (Exception e) {
            log.error("Batch interrupted", e);
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

        scheduler.shutdownNow();
        channel.removeCandleListener(listener);
        channel.close();
        BATCH_SEMAPHORE.release();

        log.info("Batch completo: {}/{} simbolos con datos", resultado.size(), symbols.size());
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
