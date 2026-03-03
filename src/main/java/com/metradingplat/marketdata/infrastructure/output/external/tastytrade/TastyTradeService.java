package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import java.time.Instant;
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
     * Siempre obtiene datos frescos de DxLink, sin cache en BD.
     */
    public Map<String, List<Candle>> getCandlesBatch(List<String> symbols, EnumTimeframe timeframe, int bars) {
        log.debug("Batch fetch: {} symbols, timeframe={}, bars={}", symbols.size(), timeframe, bars);

        Instant now = Instant.now();
        long fromTime = now.minus(timeframe.getDuration().multipliedBy(bars + 10)).toEpochMilli();

        Map<String, Long> symbolToFromTime = new HashMap<>();
        symbols.forEach(s -> symbolToFromTime.put(s, fromTime));

        return fetchCandlesBatchFromDxLink(symbols, timeframe, symbolToFromTime);
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
            Set<String> simbolosConDatos = ConcurrentHashMap.newKeySet();
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
                simbolosConDatos.add(sym);

                if (isComplete) {
                    // TX_PENDING=0: solo arrancar el settle timer cuando TODOS los símbolos
                    // pedidos enviaron al menos 1 candle. Evita que un símbolo lento (SPY)
                    // sea cortado por uno rápido (QQQ).
                    if (simbolosConDatos.containsAll(symbols)) {
                        ScheduledFuture<?> prev = settleTask.getAndSet(
                            scheduler.schedule(() -> allCompleted.complete(null), 300, TimeUnit.MILLISECONDS)
                        );
                        if (prev != null) prev.cancel(false);
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

    private void ensureConnected() {
        if (!dxLinkClient.isConnected()) {
            log.debug("Reconnecting to DxLink");
            String token = tastyTradeClient.getApiQuoteToken();
            String url = tastyTradeClient.getDxlinkUrl();
            dxLinkClient.connect(url, token);
        }
    }
}
