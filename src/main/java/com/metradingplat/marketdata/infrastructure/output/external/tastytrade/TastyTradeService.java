package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import java.time.Instant;
import java.time.LocalDate;
import java.time.temporal.ChronoUnit;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import java.util.function.BiConsumer;

import org.springframework.stereotype.Service;

import com.metradingplat.marketdata.application.output.GestionarChangeNotificationsProducerIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.ActiveEquity;
import com.metradingplat.marketdata.domain.models.BracketOrder;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.domain.models.FundamentalData;
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
     * Máx. símbolos por mensaje FEED_SUBSCRIPTION.
     * dxFeed tiene un frame WebSocket de 8 KB (8192 bytes): mensajes más grandes
     * se truncan y el servidor responde INVALID_MESSAGE (json truncado).
     * Cada item ≈ 65 bytes → 100 × 65 + 50 (wrapper) ≈ 6550 bytes < 8192.
     * Las suscripciones son aditivas al mismo canal (spec dxLink AsyncAPI 2.4.0).
     */

    private final TastyTradeClient tastyTradeClient;
    private final DxLinkClient dxLinkClient;
    private final GestionarChangeNotificationsProducerIntPort kafkaProducer;

    // Trackers para la heuristica de Halt Status (Punto 5)
    private final ConcurrentHashMap<String, Long> lastMarketDataUpdates = new ConcurrentHashMap<>();

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

            // Heuristica de Spread Anómalo (Punto 5): Evaluar spreads de error como señal adicional
            if (data.getBid() != null && data.getAsk() != null) {
                double spread = data.getAsk() - data.getBid();
                if (data.getBid() == 0 || data.getAsk() == 0 || spread < 0) {
                    log.warn("HALT HEURISTIC: Probable Suspensión para {} debido a spread anómalo (Bid={}, Ask={})", symbol, data.getBid(), data.getAsk());
                }
            }

            lastMarketDataUpdates.put(symbol, System.currentTimeMillis());
            kafkaProducer.publishMarketData(data);
        });

        // Evento determinista de Suspensión mediante canal Message (Punto 5, Refinamiento)
        dxLinkClient.setOnMessage((symbol, messageData) -> {
            // data array: [eventSymbol, eventTime, messageType, message]
            if (messageData.isArray() && messageData.size() >= 4) {
                String type = messageData.get(2).asText("");
                String text = messageData.get(3).asText("");
                log.warn("ADMIN MESSAGE [{}]: {} - {}", symbol, type, text);
                
                if (text.toLowerCase().contains("halt") || text.toLowerCase().contains("suspend")) {
                    log.error("DETERMINISTIC HALT DETECTED para {}: {}", symbol, text);
                    // Aqui se podria disparar un evento de Kafka especifico para detener la operativa
                }
            }
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
     * Obtiene candles históricos de múltiples símbolos (Punto 2).
     * Reemplaza WebSocket por llamadas REST para asegurar la alineación temporal.
     */
    public Map<String, List<Candle>> getCandlesBatch(List<String> symbols, EnumTimeframe timeframe, int bars) {
        log.info("Batch fetch REST: {} simbolos, timeframe={}, bars={}", symbols.size(), timeframe, bars);

        Instant now = Instant.now();
        Instant fromTime = now.minus(timeframe.getDuration().multipliedBy(bars));
        
        // Validación de contexto temporal (Punto 6)
        long daysBetween = java.time.Duration.between(fromTime, now).toDays();
        if (daysBetween > 270) {
            throw new IllegalArgumentException("El intervalo histórico excede los 9 meses soportados por el API.");
        }

        String tf = timeframe.getLabel(); // m, d, w, etc.
        Map<String, List<Candle>> resultado = new ConcurrentHashMap<>();
        
        symbols.parallelStream().forEach(symbol -> {
            // Construir símbolo dxFeed m{tho=true,priceType=last}:SYMBOL
            String dxSymbol = tf + "{tho=true,priceType=last}:" + symbol;
            List<Candle> candles = tastyTradeClient.getHistoricalCandles(symbol, dxSymbol, fromTime, now);
            
            if (candles != null && !candles.isEmpty()) {
                candles.forEach(c -> c.setTimeframe(timeframe));
                resultado.put(symbol, candles);
            }
        });

        log.info("Batch REST completo: {}/{} simbolos con datos", resultado.size(), symbols.size());
        return resultado;
    }

    public void shutdown() {
        log.info("Shutting down TastyTradeService...");
        dxLinkClient.disconnect();
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

    /**
     * Obtiene métricas fundamentales apoyándose únicamente en los datos
     * soportados por /market-metrics.
     */
    public Map<String, FundamentalData> getFundamentalsBatch(List<String> symbols) {
        // Normalizar simbolos a mayusculas y remover duplicados
        List<String> normalizedSymbols = symbols.stream()
                .map(String::toUpperCase)
                .distinct()
                .toList();
        
        log.info("Batch fundamentals: {} simbolos", normalizedSymbols.size());
        ensureConnected();

        DxLinkClient.DxLinkChannel channel;
        try {
            channel = dxLinkClient.openNewChannel().get(10, TimeUnit.SECONDS);
        } catch (Exception e) {
            log.error("No se pudo abrir canal DxLink para fundamentals: {}", e.getMessage());
            return Map.of();
        }

        ConcurrentHashMap<String, FundamentalData> fundamentalsMap = new ConcurrentHashMap<>();
        CompletableFuture<Void> allEssentialDataReceived = new CompletableFuture<>();
        Set<String> symbolsWithProfile = ConcurrentHashMap.newKeySet();

        BiConsumer<String, FundamentalData> listener = (sym, data) -> {
            String upperSym = sym.toUpperCase();
            fundamentalsMap.compute(upperSym, (k, v) -> {
                if (v == null) return data;
                if (data.getDayVolume() != null && data.getDayVolume() > 0) v.setDayVolume(data.getDayVolume());
                if (data.getMarketCap() != null && data.getMarketCap() > 0) v.setMarketCap(data.getMarketCap());
                if (data.getSharesOutstanding() != null && data.getSharesOutstanding() > 0) v.setSharesOutstanding(data.getSharesOutstanding());
                if (data.getFloatShares() != null && data.getFloatShares() > 0) v.setFloatShares(data.getFloatShares());
                if (data.getShortInterest() != null && data.getShortInterest() > 0) v.setShortInterest(data.getShortInterest());
                if (data.getPreMarketVolume() != null && data.getPreMarketVolume() > 0) v.setPreMarketVolume(data.getPreMarketVolume());
                if (data.getPostMarketVolume() != null && data.getPostMarketVolume() > 0) v.setPostMarketVolume(data.getPostMarketVolume());
                return v;
            });

            // Si el evento traía MarketCap o SharesOutstanding, es un evento 'Profile' (fundamental completo)
            if (data.getMarketCap() != null || data.getSharesOutstanding() != null) {
                symbolsWithProfile.add(upperSym);
                if (symbolsWithProfile.size() >= normalizedSymbols.size()) {
                    allEssentialDataReceived.complete(null);
                }
            }
        };

        channel.addFundamentalListener(listener);
        channel.subscribeFundamentalsBatch(normalizedSymbols);
        channel.subscribeExtendedVolumeBatch(normalizedSymbols);

        try {
            // Esperar hasta que todos tengan el Profile o hasta el timeout
            int maxWait = Math.min(10 + normalizedSymbols.size() / 100, 30);
            allEssentialDataReceived.get(maxWait, TimeUnit.SECONDS);
            log.info("Batch fundamentals: Todos los snapshots recibidos en tiempo record.");
        } catch (TimeoutException e) {
            log.warn("Batch fundamentals: Timeout alcanzado. Recibidos {}/{} profiles.", 
                symbolsWithProfile.size(), normalizedSymbols.size());
        } catch (Exception e) {
            log.error("Batch fundamentals error", e);
        }

        channel.close();

        // Mezclar con Market Metrics de la API REST para campos faltantes (shortRatio, earnings, IV)
        try {
            List<Map<String, Object>> metricsList = getMarketMetricsBatch(normalizedSymbols);
            for (Map<String, Object> metric : metricsList) {
                String sym = (String) metric.get("symbol");
                if (sym == null) continue;
                String finalSym = sym.toUpperCase();
                
                FundamentalData fund = fundamentalsMap.computeIfAbsent(finalSym, k -> FundamentalData.builder().symbol(k).build());

                // IV & Rank
                if (metric.get("implied-volatility-index") != null) fund.setImpliedVolatilityIndex(((Number) metric.get("implied-volatility-index")).doubleValue());
                if (metric.get("implied-volatility-index-rank") != null) fund.setImpliedVolatilityRank(((Number) metric.get("implied-volatility-index-rank")).doubleValue());
                if (metric.get("implied-volatility-percentile") != null) fund.setImpliedVolatilityPercentile(((Number) metric.get("implied-volatility-percentile")).doubleValue());
                if (metric.get("liquidity-value") != null) fund.setLiquidity(((Number) metric.get("liquidity-value")).doubleValue());
                if (metric.get("liquidity-rating") != null) fund.setLiquidityRating(((Number) metric.get("liquidity-rating")).intValue());

                // Short Data (Fallback if DxLink missed it)
                Object shortRatioValue = metric.get("short-ratio");
                if (shortRatioValue == null) shortRatioValue = metric.get("short-ratio-index");
                if (shortRatioValue instanceof Number) fund.setShortRatio(((Number) shortRatioValue).doubleValue());
                
                if (fund.getShortInterest() == null || fund.getShortInterest() == 0) {
                    Object si = metric.get("short-interest");
                    if (si instanceof Number) fund.setShortInterest(((Number) si).doubleValue());
                }

                // Earnings
                Object earningsDateValue = metric.get("earnings-report-date");
                if (earningsDateValue instanceof String) {
                    try {
                        LocalDate earningsDate = LocalDate.parse((String) earningsDateValue);
                        fund.setNextEarningsDate(earningsDate);
                        long days = ChronoUnit.DAYS.between(LocalDate.now(), earningsDate);
                        fund.setDaysUntilEarnings((int) Math.max(0, days));
                    } catch (Exception e) {
                        log.debug("Failed to parse earnings date for {}: {}", finalSym, earningsDateValue);
                    }
                }
            }
        } catch (Exception e) {
            log.error("Failed to fetch market metrics for enrichment: {}", e.getMessage());
        }

        return fundamentalsMap;
    }

    public List<Map<String, Object>> getEarningsReports(String symbol, String startDate) {
        return tastyTradeClient.getEarningsReports(symbol, startDate);
    }

    public List<Map<String, Object>> getMarketMetricsBatch(List<String> symbols) {
        log.info("Batch market metrics: {} simbolos", symbols.size());
        return tastyTradeClient.getMarketMetricsBatch(symbols);
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
