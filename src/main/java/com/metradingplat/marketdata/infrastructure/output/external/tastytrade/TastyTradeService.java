package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import java.time.Instant;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.locks.ReentrantLock;
import java.util.stream.Collectors;

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

    private final TastyTradeClient tastyTradeClient;
    private final DxLinkClient dxLinkClient;
    private final GestionarChangeNotificationsProducerIntPort kafkaProducer;

    // Lock para serializar acceso a DxLink (evita bug de callback global)
    private final ReentrantLock dxLinkLock = new ReentrantLock();

    // Cache en memoria con TTL de 55 segundos
    private final ConcurrentHashMap<String, CacheEntry> candleCache = new ConcurrentHashMap<>();
    private static final long CACHE_TTL_MS = 55_000;

    private record CacheEntry(List<Candle> candles, long timestamp) {
        boolean isExpired() {
            return System.currentTimeMillis() - timestamp > CACHE_TTL_MS;
        }
    }

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
        dxLinkClient.setOnCandle((symbol, candle) -> {
            log.debug("Candle received for {}: {} O={} H={} L={} C={}",
                    symbol, candle.getTimestamp(), candle.getOpen(),
                    candle.getHigh(), candle.getLow(), candle.getClose());
        });

        // Configurar token refresher para auto-reconexión
        dxLinkClient.setTokenRefresher(() -> {
            log.info("Token refresher called - obtaining fresh API quote token");
            return tastyTradeClient.getApiQuoteToken();
        });

        // Conectar a DxLink
        try {
            log.info("Obtaining API quote token from TastyTrade...");
            String token = tastyTradeClient.getApiQuoteToken();
            String url = tastyTradeClient.getDxlinkUrl();
            log.info("Got token and URL. Connecting to DxLink at: {}", url);
            dxLinkClient.connect(url, token);
            log.info("TastyTrade service initialized successfully");
        } catch (Exception e) {
            log.error("Failed to initialize TastyTrade service: {}", e.getMessage(), e);
            log.info("DxLinkClient will attempt auto-reconnection");
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
        log.info("Fetching candles for {} {}", symbol, timeframe);
        Map<String, List<Candle>> result = getCandlesBatch(List.of(symbol), timeframe, 700);
        return result.getOrDefault(symbol, List.of());
    }

    /**
     * Obtiene candles historicos de multiples simbolos en un solo fetch batch.
     * Usa lock para evitar el bug del callback global y cache para evitar requests redundantes.
     *
     * @param symbols lista de simbolos
     * @param timeframe timeframe de las candles
     * @param bars cantidad maxima de barras por simbolo
     * @return mapa de simbolo -> lista de candles ordenadas por timestamp asc
     */
    public Map<String, List<Candle>> getCandlesBatch(List<String> symbols, EnumTimeframe timeframe, int bars) {
        log.info("Batch fetch: {} symbols, timeframe={}, bars={}", symbols.size(), timeframe, bars);

        // Separar simbolos con cache valido de los que necesitan fetch
        Map<String, List<Candle>> resultado = new HashMap<>();
        List<String> cacheMiss = new ArrayList<>();

        for (String symbol : symbols) {
            String cacheKey = symbol + ":" + timeframe.name() + ":" + bars;
            CacheEntry entry = candleCache.get(cacheKey);
            if (entry != null && !entry.isExpired()) {
                resultado.put(symbol, entry.candles());
            } else {
                cacheMiss.add(symbol);
            }
        }

        log.info("Batch: {} cache hits, {} cache misses", resultado.size(), cacheMiss.size());

        if (cacheMiss.isEmpty()) {
            return resultado;
        }

        // Fetch de los cache-miss con lock para serializar acceso a DxLink
        Map<String, List<Candle>> fetched = fetchCandlesBatchFromDxLink(cacheMiss, timeframe, bars);

        // Guardar en cache y agregar al resultado
        for (Map.Entry<String, List<Candle>> entry : fetched.entrySet()) {
            String cacheKey = entry.getKey() + ":" + timeframe.name() + ":" + bars;
            candleCache.put(cacheKey, new CacheEntry(entry.getValue(), System.currentTimeMillis()));
            resultado.put(entry.getKey(), entry.getValue());
        }

        // Simbolos sin datos tambien se registran como lista vacia en cache
        for (String symbol : cacheMiss) {
            if (!fetched.containsKey(symbol)) {
                String cacheKey = symbol + ":" + timeframe.name() + ":" + bars;
                candleCache.put(cacheKey, new CacheEntry(List.of(), System.currentTimeMillis()));
                resultado.put(symbol, List.of());
            }
        }

        log.info("Batch complete: {} symbols total, {} con datos", resultado.size(),
            resultado.values().stream().filter(l -> !l.isEmpty()).count());

        return resultado;
    }

    private Map<String, List<Candle>> fetchCandlesBatchFromDxLink(
            List<String> symbols, EnumTimeframe timeframe, int bars) {

        dxLinkLock.lock();
        try {
            ensureConnected();

            String tf = timeframe.getLabel();
            long fromTime = Instant.now().minus(timeframe.getDuration().multipliedBy(bars + 100)).toEpochMilli();

            // Mapa concurrente para agrupar candles por simbolo
            ConcurrentHashMap<String, Set<CandleKey>> seenKeys = new ConcurrentHashMap<>();
            ConcurrentHashMap<String, List<Candle>> candlesPorSimbolo = new ConcurrentHashMap<>();

            // Configurar callback batch
            dxLinkClient.setOnCandle((sym, candle) -> {
                // sym viene como baseSymbol (extraido del candleSymbol en DxLinkClient)
                if (!symbols.contains(sym)) {
                    return;
                }
                candle.setSymbol(sym);
                candle.setTimeframe(timeframe);

                CandleKey key = new CandleKey(sym, timeframe, candle.getTimestamp());
                seenKeys.computeIfAbsent(sym, k -> ConcurrentHashMap.newKeySet());
                if (seenKeys.get(sym).add(key)) {
                    candlesPorSimbolo.computeIfAbsent(sym, k -> new ArrayList<>());
                    synchronized (candlesPorSimbolo.get(sym)) {
                        candlesPorSimbolo.get(sym).add(candle);
                    }
                }
            });

            // Construir items de suscripcion batch
            List<Map<String, Object>> subscriptionItems = symbols.stream()
                .map(symbol -> {
                    String candleSymbol = symbol + "{=" + tf + "}";
                    return Map.<String, Object>of(
                        "symbol", candleSymbol,
                        "type", "Candle",
                        "fromTime", fromTime
                    );
                })
                .toList();

            // Suscribir en batch
            dxLinkClient.resetCandleSnapshot();
            dxLinkClient.subscribeCandlesBatch(subscriptionItems);

            // Esperar estabilizacion: sin nuevas candles por 3s, con timeout escalado
            int maxWaitSeconds = Math.min(10 + symbols.size() / 20, 60);
            int noNewDataCount = 0;
            int lastTotalCount = 0;

            for (int elapsed = 0; elapsed < maxWaitSeconds; elapsed++) {
                try {
                    Thread.sleep(1000);
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                    break;
                }

                int totalCount = candlesPorSimbolo.values().stream().mapToInt(List::size).sum();

                if (elapsed % 5 == 0 || totalCount != lastTotalCount) {
                    log.info("Batch waiting... {}s/{}, {} candles totales, {} simbolos con datos",
                        elapsed + 1, maxWaitSeconds, totalCount, candlesPorSimbolo.size());
                }

                if (totalCount > 0 && totalCount == lastTotalCount) {
                    noNewDataCount++;
                    if (noNewDataCount >= 3) {
                        log.info("Batch estabilizado despues de {}s (sin datos nuevos por 3s)", elapsed + 1);
                        break;
                    }
                } else {
                    noNewDataCount = 0;
                }
                lastTotalCount = totalCount;
            }

            // Desuscribir en batch
            List<String> candleSymbols = symbols.stream()
                .map(s -> s + "{=" + tf + "}")
                .toList();
            dxLinkClient.unsubscribeCandlesBatch(candleSymbols);

            // Restaurar callback por defecto
            dxLinkClient.setOnCandle((sym, candle) -> {
                log.debug("Candle received for {}: {} O={} H={} L={} C={}",
                    sym, candle.getTimestamp(), candle.getOpen(),
                    candle.getHigh(), candle.getLow(), candle.getClose());
            });

            // Ordenar y truncar resultados
            Map<String, List<Candle>> resultado = new HashMap<>();
            for (Map.Entry<String, List<Candle>> entry : candlesPorSimbolo.entrySet()) {
                List<Candle> sorted;
                synchronized (entry.getValue()) {
                    sorted = entry.getValue().stream()
                        .sorted(Comparator.comparing(Candle::getTimestamp))
                        .collect(Collectors.toList());
                }
                // Truncar a las ultimas N barras
                if (sorted.size() > bars) {
                    sorted = sorted.subList(sorted.size() - bars, sorted.size());
                }
                resultado.put(entry.getKey(), sorted);
            }

            log.info("Batch fetch complete: {} simbolos con datos de {} solicitados",
                resultado.size(), symbols.size());

            return resultado;

        } finally {
            dxLinkLock.unlock();
        }
    }

    private record CandleKey(String symbol, EnumTimeframe timeframe, Instant timestamp) {}

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
            log.info("Reconnecting to DxLink");
            String token = tastyTradeClient.getApiQuoteToken();
            String url = tastyTradeClient.getDxlinkUrl();
            dxLinkClient.connect(url, token);
        }
    }
}
