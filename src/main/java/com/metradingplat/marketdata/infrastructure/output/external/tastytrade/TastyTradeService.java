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
import com.metradingplat.marketdata.application.output.GestionarCandleRepositoryIntPort;
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

    private final TastyTradeClient tastyTradeClient;
    private final DxLinkClient dxLinkClient;
    private final GestionarChangeNotificationsProducerIntPort kafkaProducer;
    private final GestionarCandleRepositoryIntPort candlePort;

    private static final Set<EnumTimeframe> PERMANENTES = Set.of(
            EnumTimeframe.H1, EnumTimeframe.W1, EnumTimeframe.MO1);

    @PostConstruct
    public void init() {
        log.info("Initializing TastyTrade service");

        // Configurar callback para datos de mercado → Kafka
        dxLinkClient.setOnMarketData((symbol, data) -> {
            log.debug("Market data received for {}: bid={}, ask={}, last={}",
                    symbol, data.getBid(), data.getAsk(), data.getLastPrice());
            kafkaProducer.publishMarketData(data);
        });

        // Configurar callback para candles (solo logging, no se guarda en BD)
        dxLinkClient.setOnCandle((symbol, candle, isComplete) -> {
            log.debug("Candle received for {}: {} O={} H={} L={} C={} complete={}",
                    symbol, candle.getTimestamp(), candle.getOpen(),
                    candle.getHigh(), candle.getLow(), candle.getClose(), isComplete);
        });

        // Configurar token refresher para auto-reconexión
        dxLinkClient.setTokenRefresher(() -> {
            log.info("Token refresher called - obtaining fresh API quote token");
            return tastyTradeClient.getApiQuoteToken();
        });

        // Conectar a DxLink
        try {
            log.debug("Obtaining API quote token from TastyTrade...");
            String token = tastyTradeClient.getApiQuoteToken();
            String url = tastyTradeClient.getDxlinkUrl();
            log.debug("Got token and URL. Connecting to DxLink at: {}", url);
            dxLinkClient.connect(url, token);
        } catch (Exception e) {
            log.error("Failed to initialize TastyTrade service: {}", e.getMessage(), e);
        }
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
     * Obtiene candles historicos de multiples simbolos en un solo fetch batch.
     * Utiliza PostgreSQL como caché y rellena los huecos con DxLink.
     */
    public Map<String, List<Candle>> getCandlesBatch(List<String> symbols, EnumTimeframe timeframe, int bars) {
        log.debug("Batch fetch: {} symbols, timeframe={}, bars={}", symbols.size(), timeframe, bars);

        Map<String, List<Candle>> resultado = new HashMap<>();
        List<String> cacheMiss = new ArrayList<>();
        Map<String, Long> symbolToFromTime = new HashMap<>();

        Instant now = Instant.now();
        for (String symbol : symbols) {
            long fromTime = 0L;
            boolean needsFetch = false;

            if (!PERMANENTES.contains(timeframe)) {
                // Efímero (M1/M5/M15/M30/D1): siempre pedir a DxLink, nunca usar DB
                needsFetch = true;
                fromTime = now.minus(timeframe.getDuration().multipliedBy(bars + 10)).toEpochMilli();
            } else {
                // Permanente (H1/W1/MO1): usar DB como cache con TTL = duración del timeframe
                Candle latest = candlePort.findMostRecent(symbol, timeframe);
                if (latest == null) {
                    needsFetch = true;
                    fromTime = now.minus(timeframe.getDuration().multipliedBy(bars + 10)).toEpochMilli();
                } else {
                    long ageMs = System.currentTimeMillis() - latest.getTimestamp().toEpochMilli();
                    long cacheTtl = timeframe.getDuration().toMillis(); // H1→1h, W1→7d, MO1→30d
                    if (ageMs > cacheTtl) {
                        needsFetch = true;
                        fromTime = now.minus(timeframe.getDuration().multipliedBy(bars + 10)).toEpochMilli();
                    }
                }
            }

            if (needsFetch) {
                cacheMiss.add(symbol);
                symbolToFromTime.put(symbol, fromTime);
            }
        }

        log.debug("Batch: {} symbols need fetch", cacheMiss.size());

        if (!cacheMiss.isEmpty()) {
            Map<String, List<Candle>> fetched = fetchCandlesBatchFromDxLink(cacheMiss, timeframe, symbolToFromTime);

            // Guardar asíncronamente en BD solo timeframes permanentes
            Thread.startVirtualThread(() -> {
                if (!PERMANENTES.contains(timeframe)) return;
                try {
                    int totalProcessed = 0;
                    for (List<Candle> list : fetched.values()) {
                        // Agrupar por timestamp para asegurar que si el WS nos envia
                        // la misma barra en el mismo paquete, solo guardamos una.
                        Map<Instant, Candle> uniqueBatch = new HashMap<>();
                        for (Candle c : list) {
                            uniqueBatch.put(c.getTimestamp(), c);
                        }

                        for (Candle c : uniqueBatch.values()) {
                            // La DB tiene un Unique Constraint por (symbol, timeframe, timestamp)
                            // y el upsert usa ON CONFLICT para actualizar.
                            candlePort.upsert(c);
                            totalProcessed++;
                        }
                    }
                    if (totalProcessed > 0) {
                        log.debug("Processed {} unique candles to DB", totalProcessed);
                    }
                } catch (Exception e) {
                    log.error("Error saving candles to DB", e);
                }
            });
            // Hacemos una breve pausa asumiendo que el VT escribirá rapidísimo
            // antes de hacer el read. En sistemas criticos seria un join(), pero las
            // velas que acabamos de bajar ya las tenemos en memoria ('fetched').
            // Por simplicidad, agregamos las bajadas directamente al resultado:
            for (Map.Entry<String, List<Candle>> entry : fetched.entrySet()) {
                resultado.put(entry.getKey(), entry.getValue());
            }
        }

        // Consultar la BD para obtener las velas finales ordenadas y truncadas a 'bars'
        for (String symbol : symbols) {
            List<Candle> sortedAsc = candlePort.findLatest(symbol, timeframe, bars);

            // Si el canal websocket trajo velas nuevas este milisegundo que no se han
            // comiteado al disco por el virtual thread aun, mergeamos la lista de memoria.
            if (cacheMiss.contains(symbol) && resultado.containsKey(symbol)) {
                List<Candle> memoryCandles = resultado.get(symbol);
                // Merge asegurando integridad usando Hash
                Map<Instant, Candle> merged = new HashMap<>();
                sortedAsc.forEach(c -> merged.put(c.getTimestamp(), c));
                memoryCandles.forEach(c -> merged.put(c.getTimestamp(), c));

                sortedAsc = merged.values().stream()
                        .sorted(Comparator.comparing(Candle::getTimestamp))
                        .toList();

                if (sortedAsc.size() > bars) {
                    sortedAsc = sortedAsc.subList(sortedAsc.size() - bars, sortedAsc.size());
                }
            }

            resultado.put(symbol, sortedAsc);
        }

        return resultado;
    }

    private Map<String, List<Candle>> fetchCandlesBatchFromDxLink(
            List<String> symbols, EnumTimeframe timeframe, Map<String, Long> symbolToFromTime) {

        ensureConnected();

        DxLinkClient.DxLinkChannel channel = dxLinkClient.getDefaultChannel();
        if (channel == null || !channel.isReady()) {
            throw new ResponseStatusException(HttpStatus.SERVICE_UNAVAILABLE, "DxLink default channel not ready");
        }

        DxLinkClient.CandleCallback batchListener = null;
        ScheduledExecutorService scheduler = Executors.newSingleThreadScheduledExecutor();
        try {
            String tf = timeframe.getLabel();
            ConcurrentHashMap<String, List<Candle>> candlesPorSimbolo = new ConcurrentHashMap<>();
            CompletableFuture<Void> allCompleted = new CompletableFuture<>();
            AtomicReference<ScheduledFuture<?>> settleTask = new AtomicReference<>();

            batchListener = (sym, candle, isComplete) -> {
                if (!symbols.contains(sym))
                    return;

                candle.setTimeframe(timeframe);
                candlesPorSimbolo.computeIfAbsent(sym, k -> new ArrayList<>());
                synchronized (candlesPorSimbolo.get(sym)) {
                    candlesPorSimbolo.get(sym).add(candle);
                }

                if (isComplete) {
                    // TX_PENDING=0: fin del lote actual. Si no llegan más candles en
                    // 300ms, el snapshot está completo.
                    ScheduledFuture<?> prev = settleTask.getAndSet(
                        scheduler.schedule(() -> allCompleted.complete(null), 300, TimeUnit.MILLISECONDS)
                    );
                    if (prev != null) prev.cancel(false);
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
                log.warn("Batch timed out after {}s on shared channel {}. Symbols: {}",
                        maxWaitSeconds, channel.getId(), symbols);
                throw new ResponseStatusException(HttpStatus.SERVICE_UNAVAILABLE,
                        "DxLink batch timed out. Symbols: " + symbols, e);
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
            if (channel != null && batchListener != null) {
                channel.removeCandleListener(batchListener);
                // Opcional: Desuscribir estos simbolos para no seguir recibiendo updates en el
                // canal compartido si no son real-time
                // Pero ojo, si son simbolos que estan en live-streaming no queremos quitarlos.
                // En esta arquitectura, el live streaming usa simbolos sin el {=tf}.
                // Las velas historicas usan {=tf}. Asi que es seguro desuscribir.
                final DxLinkClient.DxLinkChannel finalChannel = channel;
                symbols.forEach(s -> {
                    String candleSymbol = s + "{=" + timeframe.getLabel() + "}";
                    finalChannel.unsubscribe(candleSymbol); // Unsubscribe uses generic removal
                });
            }
        }
    }

    /**
     * Obtiene candles actuales sin cache (Bypass Db)
     */
    public Map<String, List<Candle>> getCandlesBatchNoCache(List<String> symbols, EnumTimeframe timeframe, int bars) {
        Map<String, Long> symbolToFromTime = new HashMap<>();
        long fromTime = Instant.now().minus(timeframe.getDuration().multipliedBy(bars + 10)).toEpochMilli();
        symbols.forEach(s -> symbolToFromTime.put(s, fromTime));
        return fetchCandlesBatchFromDxLink(symbols, timeframe, symbolToFromTime);
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

    /**
     * Actualiza candles para una lista de símbolos. Primera vez: obtiene todo el histórico.
     * Ejecuciones posteriores: solo desde la última barra almacenada. Solo guarda barras completas.
     * Usado por el cron de mantenimiento.
     */
    public Map<String, List<Candle>> refreshCandles(List<String> symbols, EnumTimeframe timeframe, int barsHistoricoInicial) {
        Map<String, Long> symbolToFromTime = new HashMap<>();
        Instant now = Instant.now();
        for (String symbol : symbols) {
            Candle latest = candlePort.findMostRecent(symbol, timeframe);
            long fromTime = (latest == null)
                    ? now.minus(timeframe.getDuration().multipliedBy(barsHistoricoInicial)).toEpochMilli()
                    : latest.getTimestamp().toEpochMilli();
            symbolToFromTime.put(symbol, fromTime);
        }
        Map<String, List<Candle>> fetched = fetchCandlesBatchFromDxLink(symbols, timeframe, symbolToFromTime);
        Thread.startVirtualThread(() -> {
            int saved = 0;
            try {
                for (Map.Entry<String, List<Candle>> entry : fetched.entrySet()) {
                    for (Candle c : entry.getValue()) {
                        if (c.getTimestamp().plus(timeframe.getDuration()).isBefore(now)) {
                            candlePort.upsert(c);
                            saved++;
                        }
                    }
                }
                log.info("refreshCandles {}: {} barras completas guardadas ({} simbolos)",
                        timeframe, saved, symbols.size());
            } catch (Exception e) {
                log.error("Error guardando candles en refreshCandles {}", timeframe, e);
            }
        });
        return fetched;
    }

    /**
     * Comprueba si existe una nueva barra completada en DxLink respecto a lo que hay en DB.
     * Usado como probe rápido antes de lanzar la actualización masiva del cron.
     */
    public boolean hayNuevaBarraCompletada(String simbolo, EnumTimeframe timeframe) {
        try {
            Candle latest = candlePort.findMostRecent(simbolo, timeframe);
            Map<String, List<Candle>> fetched = getCandlesBatchNoCache(List.of(simbolo), timeframe, 1);
            List<Candle> bars = fetched.getOrDefault(simbolo, List.of());
            Instant now = Instant.now();
            return bars.stream()
                    .filter(c -> c.getTimestamp().plus(timeframe.getDuration()).isBefore(now))
                    .anyMatch(c -> latest == null || c.getTimestamp().isAfter(latest.getTimestamp()));
        } catch (Exception e) {
            log.warn("Probe {} {} falló: {}", simbolo, timeframe, e.getMessage());
            return false;
        }
    }

    /**
     * Elimina de la DB los timeframes no permanentes (M1, M5, M15, M30, D1).
     * Usado por la limpieza diaria del cron.
     */
    public void limpiarTimeframesObsoletos() {
        candlePort.deleteByTimeframesNotIn(new ArrayList<>(PERMANENTES));
        log.info("Limpieza: eliminados timeframes no permanentes (se conservan H1, W1, MO1)");
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
