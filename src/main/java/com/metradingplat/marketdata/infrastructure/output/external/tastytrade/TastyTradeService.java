package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import java.time.Instant;
import java.time.LocalDate;
import java.time.temporal.ChronoUnit;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;

import org.springframework.stereotype.Service;

import com.metradingplat.marketdata.application.output.GestionarChangeNotificationsProducerIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.ActiveEquity;
import com.metradingplat.marketdata.domain.models.BracketOrder;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.domain.models.FundamentalData;
import com.metradingplat.marketdata.domain.models.OptionChain;
import com.metradingplat.marketdata.domain.models.OptionContract;
import com.metradingplat.marketdata.domain.models.OrderRequest;
import com.metradingplat.marketdata.domain.models.OrderResponse;
import com.metradingplat.marketdata.infrastructure.output.external.finra.FinraClient;

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
    private final AccountStreamerClient accountStreamerClient;
    private final GestionarChangeNotificationsProducerIntPort kafkaProducer;
    private final FinraClient finraClient;
    private final java.util.concurrent.ScheduledExecutorService scheduler = java.util.concurrent.Executors.newSingleThreadScheduledExecutor();

    // Trackers para la heuristica de Halt Status (Punto 5)
    private final ConcurrentHashMap<String, Long> lastMarketDataUpdates = new ConcurrentHashMap<>();
    private final ConcurrentHashMap<String, OptionContract> greeksCache = new ConcurrentHashMap<>();

    // Cachés Globales L1 (Arquitectura de Resiliencia)
    private final ConcurrentHashMap<String, FundamentalData> fundamentalsCache = new ConcurrentHashMap<>();
    private final ConcurrentHashMap<String, Double> lastPricesCache = new ConcurrentHashMap<>();
    private final ConcurrentHashMap<String, Map<String, Object>> positionsCache = new ConcurrentHashMap<>();
    private final ConcurrentHashMap<String, Map<String, Object>> liveOrdersCache = new ConcurrentHashMap<>();

    public Map<String, FundamentalData> getCachedFundamentals(List<String> symbols) {
        Map<String, FundamentalData> result = new ConcurrentHashMap<>();
        for (String sym : symbols) {
            String upper = sym.toUpperCase();
            FundamentalData cached = fundamentalsCache.get(upper);
            if (cached != null) result.put(upper, cached);
        }
        return result;
    }

    @PostConstruct
    public void init() {
        log.info("Initializing TastyTrade service and Synchronizing State...");
        
        // 1. Sincronización de Estado (Truth from Broker)
        reconcileAccountState();

        // 2. Configurar streams

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
            
            if (token != null && url != null) {
                log.debug("Got token and URL. Connecting to DxLink at: {}", url);
                dxLinkClient.connect(url, token);
            } else {
                log.warn("TastyTrade token or URL is null. Skipping DxLink connection (likely in test environment).");
            }
        } catch (Exception e) {
            log.error("Failed to initialize TastyTrade service: {}", e.getMessage(), e);
        }

        // Configurar listener de fundamentales en el canal DEFAULT
        // Usa setOnFundamentalData que se re-aplica en cada reconnect
        dxLinkClient.setOnFundamentalData((sym, data) -> {
            String upperSym = sym.toUpperCase();
            fundamentalsCache.merge(upperSym, data, (v1, v2) -> {
                if (v2.getSharesOutstanding() != null) v1.setSharesOutstanding(v2.getSharesOutstanding());
                if (v2.getFloatShares() != null) v1.setFloatShares(v2.getFloatShares());
                if (v2.getEps() != null) v1.setEps(v2.getEps());
                if (v2.getDividendAmount() != null) v1.setDividendAmount(v2.getDividendAmount());
                if (v2.getDividendFrequency() != null) v1.setDividendFrequency(v2.getDividendFrequency());
                if (v2.getTradingStatus() != null) v1.setTradingStatus(v2.getTradingStatus());
                if (v2.getStatusReason() != null) v1.setStatusReason(v2.getStatusReason());
                if (v2.getHaltStartTime() != null) v1.setHaltStartTime(v2.getHaltStartTime());
                if (v2.getHaltEndTime() != null) v1.setHaltEndTime(v2.getHaltEndTime());
                if (v2.getBeta() != null) v1.setBeta(v2.getBeta());
                if (v2.getOpen() != null) v1.setOpen(v2.getOpen());
                if (v2.getHigh() != null) v1.setHigh(v2.getHigh());
                if (v2.getLow() != null) v1.setLow(v2.getLow());
                if (v2.getPrevClose() != null) v1.setPrevClose(v2.getPrevClose());
                if (v2.getOpenInterest() != null) v1.setOpenInterest(v2.getOpenInterest());
                if (v2.getPreMarketVolume() != null) v1.setPreMarketVolume(v2.getPreMarketVolume());
                if (v2.getPostMarketVolume() != null) v1.setPostMarketVolume(v2.getPostMarketVolume());
                if (v2.getImpliedVolatilityIndex() != null) v1.setImpliedVolatilityIndex(v2.getImpliedVolatilityIndex());
                if (v2.getImpliedVolatilityRank() != null) v1.setImpliedVolatilityRank(v2.getImpliedVolatilityRank());
                if (v2.getImpliedVolatilityPercentile() != null) v1.setImpliedVolatilityPercentile(v2.getImpliedVolatilityPercentile());
                if (v2.getLiquidity() != null) v1.setLiquidity(v2.getLiquidity());
                if (v2.getLiquidityRating() != null) v1.setLiquidityRating(v2.getLiquidityRating());
                return v1;
            });
        });

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

            if (data.getLastPrice() != null && data.getLastPrice() > 0) {
                lastPricesCache.put(symbol.toUpperCase(), data.getLastPrice());
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

        dxLinkClient.setOnGreeks((symbol, greeks) -> {
            Double iv = greeks.getImpliedVolatility();
            if (iv != null && iv > 1.0) { // Señal de alta volatilidad (>100% IV)
                log.warn("🔥 [ALPHA SIGNAL] Volatilidad Extrema en {}: IV={}% | Delta={}", 
                        symbol, Math.round(iv * 100), greeks.getDelta());
            }
            
            greeksCache.put(symbol, greeks);
        });

        log.info("DxLink initialized. Auto-subscribe and REST preload will start in 5s...");
        CompletableFuture.runAsync(() -> {
            try {
                Thread.sleep(5000);
                if (dxLinkClient.isConnected()) {
                    subscribeAllMarkets();
                } else {
                    log.warn("DxLink not connected, running REST-only preload");
                    List<ActiveEquity> allEquities = tastyTradeClient.getAllActiveEquities();
                    List<String> symbols = allEquities.stream()
                            .map(ActiveEquity::getSymbol)
                            .distinct()
                            .toList();
                    log.info("REST-only preload for {} symbols (no DxLink)", symbols.size());
                    preloadFundamentalsFromRest(symbols);
                }
            } catch (Exception e) {
                log.error("Auto-subscribe/preload failed: {}", e.getMessage());
            }
        });
    }

    public void sendOrder(OrderRequest request) {
        log.info("Sending order: {} {} {} @ {}",
                request.getAction(), request.getQuantity(), request.getSymbol(), request.getPrice());
        tastyTradeClient.submitOrder(request);
    }

    private static final List<String> ALL_US_MARKETS = List.of("XNAS", "XNYS", "XASE", "ARCX", "BATS", "OTC");

    private void subscribeAllMarkets() {
        log.info("Auto-subscribing to all US equity markets: {}", ALL_US_MARKETS);
        List<ActiveEquity> allEquities = tastyTradeClient.getAllActiveEquities();
        Set<String> targetMarkets = new HashSet<>(ALL_US_MARKETS);
        List<String> symbols = new ArrayList<>();
        for (ActiveEquity eq : allEquities) {
            if (eq.getListedMarket() != null && targetMarkets.contains(eq.getListedMarket().toUpperCase())) {
                symbols.add(eq.getSymbol());
            }
        }
        if (!symbols.isEmpty()) {
            List<String> distinctSymbols = symbols.stream().distinct().toList();
            subscribeBatch(distinctSymbols);
            log.info("Auto-subscribed {} symbols for real-time prices. Starting fundamentals preload...", distinctSymbols.size());
            CompletableFuture.runAsync(() -> preloadFundamentalsFromRest(distinctSymbols));
        }
    }

    private void preloadFundamentalsFromRest(List<String> symbols) {
        log.info("Bulk preloading REST fundamentals for {} symbols", symbols.size());
        int chunkSize = 250;
        int loaded = 0;
        int emptyChunks = 0;

        for (int i = 0; i < symbols.size(); i += chunkSize) {
            int end = Math.min(i + chunkSize, symbols.size());
            List<String> chunk = symbols.subList(i, end);
            try {
                List<Map<String, Object>> metrics = tastyTradeClient.getMarketMetricsBatch(chunk);
                if (metrics.isEmpty()) {
                    emptyChunks++;
                }
                for (Map<String, Object> m : metrics) {
                    String sym = (String) m.get("symbol");
                    if (sym == null) continue;
                    FundamentalData fund = fundamentalsCache.computeIfAbsent(sym.toUpperCase(), k -> FundamentalData.builder().symbol(k).build());
                    if (fund.getBeta() == null) fund.setBeta(safeConvertToDouble(m.get("beta")));
                    if (fund.getEps() == null) fund.setEps(safeConvertToDouble(m.get("earnings-per-share")));
                    if (fund.getMarketCap() == null) fund.setMarketCap(safeConvertToDouble(m.get("market-cap")));
                    if (fund.getShortRatio() == null) fund.setShortRatio(safeConvertToDouble(m.get("short-ratio")));
                    if (fund.getDividendAmount() == null) fund.setDividendAmount(safeConvertToDouble(m.get("dividend-rate-per-share")));
                    fund.setBorrowRate(safeConvertToDouble(m.get("borrow-rate")));
                    fund.setLendability((String) m.get("lendability"));
                    Object earnObj = m.get("earnings");
                    if (earnObj instanceof Map<?,?> earnMap && fund.getNextEarningsDate() == null) {
                        Object earnDate = earnMap.get("expected-report-date");
                        if (earnDate instanceof String dateStr) {
                            try {
                                fund.setNextEarningsDate(LocalDate.parse(dateStr));
                                long days = java.time.temporal.ChronoUnit.DAYS.between(LocalDate.now(), LocalDate.parse(dateStr));
                                fund.setDaysUntilEarnings((int) Math.max(0, days));
                            } catch (Exception ignored) {}
                        } else if (loaded < 3) {
                            log.info("Earnings object for {}: keys={}, estimated-report-date type={}",
                                    sym, earnMap.keySet(), earnDate == null ? "null" : earnDate.getClass().getSimpleName());
                        }
                    } else if (earnObj != null && loaded < 3) {
                        log.info("Earnings field for {}: type={}, value={}", sym, earnObj.getClass().getSimpleName(), earnObj);
                    }
                    fund.setImpliedVolatilityIndex(safeConvertToDouble(m.get("implied-volatility-index")));
                    fund.setImpliedVolatilityRank(safeConvertToDouble(m.get("implied-volatility-index-rank")));
                    fund.setImpliedVolatilityPercentile(safeConvertToDouble(m.get("implied-volatility-percentile")));
                    fund.setLiquidity(safeConvertToDouble(m.get("liquidity-value")));
                    fund.setLiquidityRating(safeConvertToInt(m.get("liquidity-rating")));
                    loaded++;
                }
            } catch (Exception e) {
                log.warn("Preload market-metrics chunk at {} failed: {}", i, e.getMessage());
            }
        }
        log.info("Preload market-metrics: {} loaded, {} empty chunks", loaded, emptyChunks);

        int equityLoaded = 0;
        for (int i = 0; i < symbols.size(); i += chunkSize) {
            int end = Math.min(i + chunkSize, symbols.size());
            try {
                List<Map<String, Object>> equities = tastyTradeClient.getEquitiesBatch(symbols.subList(i, end));
                if (i == 0 && !equities.isEmpty()) {
                    log.info("Equities keys sample for {}: {}", equities.get(0).get("symbol"), equities.get(0).keySet());
                }
                for (Map<String, Object> eq : equities) {
                    String sym = (String) eq.get("symbol");
                    if (sym == null) continue;
                    FundamentalData fund = fundamentalsCache.computeIfAbsent(sym.toUpperCase(), k -> FundamentalData.builder().symbol(k).build());
                    if (fund.getSharesOutstanding() == null) fund.setSharesOutstanding(safeConvertToLong(eq.get("shares-outstanding")));
                    if (fund.getFloatShares() == null) fund.setFloatShares(safeConvertToLong(eq.get("free-float")));
                    if (fund.getBeta() == null) fund.setBeta(safeConvertToDouble(eq.get("beta")));
                    equityLoaded++;
                }
            } catch (Exception e) {
                log.warn("Preload equities chunk at {} failed: {}", i, e.getMessage());
            }
        }
        log.info("Preload equities: {} loaded", equityLoaded);

        int ohlcLoaded = 0;
        int ohlcChunkSize = 100;
        for (int i = 0; i < symbols.size(); i += ohlcChunkSize) {
            int end = Math.min(i + ohlcChunkSize, symbols.size());
            try {
                List<Map<String, Object>> ohlc = tastyTradeClient.getMarketDataBatch(symbols.subList(i, end));
                for (Map<String, Object> item : ohlc) {
                    String sym = (String) item.get("symbol");
                    if (sym == null) continue;
                    FundamentalData fund = fundamentalsCache.computeIfAbsent(sym.toUpperCase(), k -> FundamentalData.builder().symbol(k).build());
                    if (fund.getOpen() == null) fund.setOpen(safeConvertToDouble(item.get("open")));
                    if (fund.getHigh() == null) fund.setHigh(safeConvertToDouble(item.get("high")));
                    if (fund.getLow() == null) fund.setLow(safeConvertToDouble(item.get("low")));
                    if (fund.getPrevClose() == null) fund.setPrevClose(safeConvertToDouble(item.get("prev-close")));
                    if (fund.getMarketCap() == null) fund.setMarketCap(safeConvertToDouble(item.get("market-cap")));
                    ohlcLoaded++;
                }
            } catch (Exception e) {
                log.warn("Preload OHLC chunk at {} failed: {}", i, e.getMessage());
            }
        }
        log.info("Preload OHLC: {} loaded", ohlcLoaded);

        log.info("Bulk preload REST totals: market-metrics={}, equities={}, ohlc={}. Starting DxLink phase in background.", loaded, equityLoaded, ohlcLoaded);

        int calculatedShares = 0;
        int calculatedFloat = 0;
        for (String sym : symbols) {
            FundamentalData fund = fundamentalsCache.get(sym.toUpperCase());
            if (fund == null) continue;
            Double price = fund.getPrevClose();
            if (price == null) price = fund.getOpen();

            if (fund.getSharesOutstanding() == null
                    && price != null && price > 0
                    && fund.getMarketCap() != null && fund.getMarketCap() > 0) {
                long shares = Math.round(fund.getMarketCap() / price);
                if (shares > 0) {
                    fund.setSharesOutstanding(shares);
                    calculatedShares++;
                }
            }

            if (fund.getFloatShares() == null
                    && fund.getSharesOutstanding() != null
                    && fund.getSharesOutstanding() > 0) {
                long estimatedFloat = Math.round(fund.getSharesOutstanding() * 0.90);
                fund.setFloatShares(estimatedFloat);
                calculatedFloat++;
            }
        }
        log.info("Calculated sharesOutstanding for {} symbols, floatShares for {} symbols",
                calculatedShares, calculatedFloat);

        CompletableFuture.runAsync(this::updateShortInterestFromFinra);
        CompletableFuture.runAsync(() -> {
            List<String> stillMissing = new ArrayList<>();
            for (String sym : symbols) {
                FundamentalData fund = fundamentalsCache.get(sym.toUpperCase());
                if (fund == null || fund.getBeta() == null) {
                    stillMissing.add(sym);
                }
            }
            if (!stillMissing.isEmpty()) {
                log.info("DxLink ephemeral background preload for {} symbols", stillMissing.size());
                try {
                    getFundamentalsBatch(stillMissing);
                    log.info("DxLink ephemeral background preload complete");
                } catch (Exception e) {
                    log.warn("DxLink ephemeral background preload failed: {}", e.getMessage());
                }
            }
        });
    }

    private static final int SUBSCRIBE_CHUNK_SIZE = 33;
    private void updateShortInterestFromFinra() {
        log.info("Downloading FINRA short interest data...");
        try {
            Map<String, FinraClient.ShortInterestRecord> finraData = finraClient.downloadLatest();
            if (finraData.isEmpty()) return;

            int updated = 0;
            for (var entry : finraData.entrySet()) {
                String sym = entry.getKey();
                FinraClient.ShortInterestRecord rec = entry.getValue();
                FundamentalData fund = fundamentalsCache.computeIfAbsent(sym,
                        k -> FundamentalData.builder().symbol(k).build());

                fund.setShortRatio(rec.daysToCover > 0 ? rec.daysToCover : null);
                fund.setDayVolume(rec.avgDailyVolume > 0 ? rec.avgDailyVolume : null);

                if (fund.getFloatShares() != null && fund.getFloatShares() > 0 && rec.sharesShorted > 0) {
                    double shortPct = (double) rec.sharesShorted / fund.getFloatShares() * 100.0;
                    fund.setShortInterest(Math.round(shortPct * 100.0) / 100.0);
                }
                updated++;
            }
            log.info("FINRA short interest updated for {} symbols", updated);
        } catch (Exception e) {
            log.warn("FINRA short interest update failed: {}", e.getMessage());
        }
    }

    private static final long SUBSCRIBE_CHUNK_DELAY_MS = 500;

    public void subscribeBatch(List<String> symbols) {
        log.info("Batch subscribing {} symbols (chunks of {}, {}ms delay)", symbols.size(), SUBSCRIBE_CHUNK_SIZE, SUBSCRIBE_CHUNK_DELAY_MS);
        ensureConnected();
        if (!dxLinkClient.isConnected()) {
            log.error("Cannot subscribe: DxLink not connected");
            return;
        }
        for (int i = 0; i < symbols.size(); i += SUBSCRIBE_CHUNK_SIZE) {
            int end = Math.min(i + SUBSCRIBE_CHUNK_SIZE, symbols.size());
            dxLinkClient.subscribeBatch(symbols.subList(i, end));
            if (end < symbols.size()) {
                try { Thread.sleep(SUBSCRIBE_CHUNK_DELAY_MS); } catch (InterruptedException e) { Thread.currentThread().interrupt(); break; }
            }
        }
        int active = dxLinkClient.getActiveSubscriptionCount();
        log.info("Batch subscribe done: requested {} symbols, {} now active", symbols.size(), active);
    }

    public void unsubscribeBatch(List<String> symbols) {
        log.info("Batch unsubscribing {} symbols from DxLink", symbols.size());
        for (String sym : symbols) {
            dxLinkClient.unsubscribe(sym);
        }
        log.info("Batch unsubscribe complete: {} symbols removed", symbols.size());
    }

    public int getActiveSubscriptionCount() {
        return dxLinkClient.getActiveSubscriptionCount();
    }

    public Map<String, Double> getCachedPrices(List<String> symbols) {
        Map<String, Double> result = new ConcurrentHashMap<>();
        for (String sym : symbols) {
            String upper = sym.toUpperCase();
            Double price = lastPricesCache.get(upper);
            if (price != null) result.put(upper, price);
        }
        return result;
    }

    public void subscribe(String symbol) {
        log.info("Subscribing to real-time data and Alpha Metrics: {}", symbol);
        ensureConnected();

        // 1. Ingesta de Datos (dxLink)
        dxLinkClient.subscribe(symbol);
        
        // 2. Enriquecimiento Alpha (REST Market Metrics)
        CompletableFuture.runAsync(() -> {
            try {
                List<Map<String, Object>> metrics = tastyTradeClient.getMarketMetricsBatch(List.of(symbol));
                if (!metrics.isEmpty()) {
                    Map<String, Object> m = metrics.get(0);
                    log.info("📊 Alpha Metrics hídricas para {}: IV Rank={}", symbol, m.get("implied-volatility-index-rank"));
                    
                    String upperSym = symbol.toUpperCase();
                    FundamentalData fund = fundamentalsCache.computeIfAbsent(upperSym, k -> FundamentalData.builder().symbol(k).build());
                    fund.setImpliedVolatilityIndex(safeConvertToDouble(m.get("implied-volatility-index")));
                    fund.setImpliedVolatilityRank(safeConvertToDouble(m.get("implied-volatility-index-rank")));
                    fund.setImpliedVolatilityPercentile(safeConvertToDouble(m.get("implied-volatility-percentile")));
                    fund.setLiquidity(safeConvertToDouble(m.get("liquidity-value")));
                    fund.setLiquidityRating(safeConvertToInt(m.get("liquidity-rating")));
                    fund.setShortRatio(safeConvertToDouble(m.get("short-ratio")));
                }
            } catch (Exception e) {
                log.warn("Falla silenciosa en Alpha Enrichment para {}", symbol);
            }
        });
    }

    public void unsubscribe(String symbol) {
        log.info("Unsubscribing from: {}", symbol);
        dxLinkClient.unsubscribe(symbol);
    }

    /**
     * Suscribe a velas históricas + live streaming (Time-Series Engine).
     */
    public void subscribeToCandles(String symbol, EnumTimeframe timeframe, int daysBack) {
        log.info("📊 Subscribing to Time-Series for {}: {} ({} days back)", 
                symbol, timeframe, daysBack);
        ensureConnected();
        
        long fromTime = Instant.now().minus(daysBack, ChronoUnit.DAYS).toEpochMilli();
        // El formato dxLinkFormat ya trae "{=1m}", extraemos solo el valor "1m"
        String tf = timeframe.getDxLinkFormat().replace("{=", "").replace("}", "");
        
        dxLinkClient.getDefaultChannel().subscribeCandlesHistory(symbol, tf, fromTime);
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
        log.info("Batch fetch WebSocket History: {} simbolos, timeframe={}, bars={}", symbols.size(), timeframe, bars);

        Instant now = Instant.now();
        // Sobre-aproximamos el tiempo (x1.5) para cubrir fines de semana y feriados
        Instant fromTime = now.minus(timeframe.getDuration().multipliedBy((long)(bars * 1.5)));
        
        // Validación de contexto temporal (Punto 6)
        long daysBetween = java.time.Duration.between(fromTime, now).toDays();
        boolean isIntraday = timeframe.getLabel().contains("m") || timeframe.getLabel().contains("h");
        
        if (isIntraday && daysBetween > 270) {
            // Si el x1.5 se pasa de los 9 meses, bajamos al límite máximo permitido
            fromTime = now.minus(270, java.time.temporal.ChronoUnit.DAYS);
        } else if (daysBetween > 3650) { 
            fromTime = now.minus(3650, java.time.temporal.ChronoUnit.DAYS);
        }

        String label = timeframe.getLabel(); // ej: "5m", "1d"
        String type = label.substring(label.length() - 1); // "m", "d", etc.
        String period = label.substring(0, label.length() - 1); // "5", "1", etc.

        ensureConnected();
        
        Map<String, List<Candle>> resultado = new ConcurrentHashMap<>();
        CompletableFuture<Map<String, List<Candle>>> future = new CompletableFuture<>();
        
        try {
            DxLinkClient.DxLinkChannel channel = dxLinkClient.openNewChannel().get(10, TimeUnit.SECONDS);
            
            AtomicInteger receivedSnapshots = new AtomicInteger(0);
            AtomicInteger expectedSnapshots = new AtomicInteger(symbols.size());

            channel.addCandleListener((symbol, candle, isSnapshotComplete) -> {
                String cleanSymbol = symbol.replaceAll("\\{=.*\\}", "");
                log.info("Ephemeral candle channel {}: received {} O={} C={} complete={}",
                    channel.getId(), cleanSymbol, candle.getOpen(), candle.getClose(), isSnapshotComplete);
                candle.setTimeframe(timeframe);
                resultado.computeIfAbsent(cleanSymbol, k -> new java.util.ArrayList<>()).add(candle);
                if (isSnapshotComplete && receivedSnapshots.incrementAndGet() >= expectedSnapshots.get()) {
                    log.info("All candle snapshots received ({}), completing future", receivedSnapshots.get());
                    scheduler.schedule(() -> {
                        channel.close();
                        resultado.forEach((sym, candles) -> {
                            candles.sort(java.util.Comparator.comparing(Candle::getTimestamp));
                            if (candles.size() > bars) {
                                resultado.put(sym, new java.util.ArrayList<>(candles.subList(candles.size() - bars, candles.size())));
                            }
                        });
                        future.complete(resultado);
                    }, 100, TimeUnit.MILLISECONDS);
                }
            });

            List<Map<String, Object>> subscriptionItems = symbols.stream()
                .map(s -> {
                    String dxSymbol = String.format("%s{=%s%s}", s, period, type);
                    Map<String, Object> item = new java.util.HashMap<>();
                    item.put("symbol", dxSymbol);
                    item.put("type", "Candle");
                    return item;
                })
                .toList();

            channel.subscribeCandlesHistory(subscriptionItems, fromTime.toEpochMilli());

            scheduler.schedule(() -> {
                if (!future.isDone()) {
                    channel.close();
                    resultado.forEach((symbol, candles) -> {
                        candles.sort(java.util.Comparator.comparing(Candle::getTimestamp));
                        if (candles.size() > bars) {
                            resultado.put(symbol, new java.util.ArrayList<>(candles.subList(candles.size() - bars, candles.size())));
                        }
                    });
                    future.complete(resultado);
                }
            }, 10 + (symbols.size() / 2), TimeUnit.SECONDS);

            return future.get(15, TimeUnit.SECONDS);

        } catch (Exception e) {
            log.error("Failed to fetch candles via WebSocket", e);
            return Map.of();
        }
    }

    public OptionChain getOptionChain(String symbol) {
        log.info("Fetching option chain for {}", symbol);
        Map<String, Object> nested = tastyTradeClient.getOptionChainNested(symbol);
        if (nested == null || nested.isEmpty()) return null;

        OptionChain chain = OptionChain.builder()
                .symbol(symbol)
                .expirations(new java.util.HashMap<>())
                .build();

        List<String> allOptionSymbols = new java.util.ArrayList<>();

        try {
            @SuppressWarnings("unchecked")
            List<Map<String, Object>> expirations = (List<Map<String, Object>>) nested.get("expirations");
            if (expirations != null) {
                for (Map<String, Object> exp : expirations) {
                    String date = (String) exp.get("expiration-date");
                    @SuppressWarnings("unchecked")
                    List<Map<String, Object>> strikes = (List<Map<String, Object>>) exp.get("strikes");
                    
                    List<OptionContract> contracts = new java.util.ArrayList<>();
                    if (strikes != null) {
                        for (Map<String, Object> strike : strikes) {
                            addContract(contracts, allOptionSymbols, symbol, date, strike, "call");
                            addContract(contracts, allOptionSymbols, symbol, date, strike, "put");
                        }
                    }
                    chain.getExpirations().put(date, contracts);
                }
            }
        } catch (Exception e) {
            log.error("Error parsing option chain for {}: {}", symbol, e.getMessage());
        }

        // Suscripción masiva por lotes (Batch)
        if (!allOptionSymbols.isEmpty()) {
            log.info("Subscribing to Greeks for {} options of {}", allOptionSymbols.size(), symbol);
            ensureConnected();
            // Dividir en lotes de 100 para no exceder los 8KB del frame dxLink
            for (int i = 0; i < allOptionSymbols.size(); i += 100) {
                List<String> chunk = allOptionSymbols.subList(i, Math.min(i + 100, allOptionSymbols.size()));
                dxLinkClient.subscribe(chunk);
                try { Thread.sleep(50); } catch (InterruptedException ignored) {} // Throttle
            }
        }

        return chain;
    }

    private void addContract(List<OptionContract> list, List<String> allSymbols, String root, String date, Map<String, Object> strike, String side) {
        @SuppressWarnings("unchecked")
        Map<String, Object> data = (Map<String, Object>) strike.get(side);
        if (data != null) {
            String osiSymbol = (String) data.get("streamer-symbol");
            if (osiSymbol != null) {
                OptionContract cached = greeksCache.get(osiSymbol);
                OptionContract contract = OptionContract.builder()
                        .symbol(osiSymbol)
                        .rootSymbol(root)
                        .expirationDate(LocalDate.parse(date))
                        .strikePrice(((Number) strike.get("strike-price")).doubleValue())
                        .optionType(side.toUpperCase())
                        .delta(cached != null ? cached.getDelta() : null)
                        .gamma(cached != null ? cached.getGamma() : null)
                        .theta(cached != null ? cached.getTheta() : null)
                        .vega(cached != null ? cached.getVega() : null)
                        .rho(cached != null ? cached.getRho() : null)
                        .impliedVolatility(cached != null ? cached.getImpliedVolatility() : null)
                        .theoreticalPrice(cached != null ? cached.getTheoreticalPrice() : null)
                        .build();
                list.add(contract);
                allSymbols.add(osiSymbol);
            }
        }
    }

    public void shutdown() {
        log.info("Shutting down TastyTradeService...");
        dxLinkClient.disconnect();
        scheduler.shutdown();
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

    public List<Map<String, Object>> getEquitiesBatch(List<String> symbols) {
        return tastyTradeClient.getEquitiesBatch(symbols);
    }

    public List<Map<String, Object>> getMarketDataBatch(List<String> symbols) {
        return tastyTradeClient.getMarketDataBatch(symbols);
    }

    public Map<String, Object> getMarketDataByType(String symbol) {
        return tastyTradeClient.getMarketDataByType(symbol);
    }

    /**
     * Obtiene métricas fundamentales apoyándose en dxLink (Profile/Summary) 
     * y enriqueciendo con Market Metrics de la API REST.
     */
    public Map<String, FundamentalData> getFundamentalsBatch(List<String> symbols) {
        List<String> normalizedSymbols = symbols.stream()
                .map(String::toUpperCase)
                .distinct()
                .toList();
        
        log.info("Batch fundamentals: {} simbolos via dxLink", normalizedSymbols.size());
        ensureConnected();
        
        CompletableFuture<Void> snapshotReceived = new CompletableFuture<>();
        Set<String> symbolsWithProfile = ConcurrentHashMap.newKeySet();

        try (DxLinkClient.DxLinkChannel channel = dxLinkClient.openNewChannel().join()) {
            channel.addFundamentalListener((sym, data) -> {
                String upperSym = sym.toUpperCase();
                fundamentalsCache.merge(upperSym, data, (v1, v2) -> {
                    // Combinar datos de Profile y Summary
                    if (v2.getSharesOutstanding() != null) v1.setSharesOutstanding(v2.getSharesOutstanding());
                    if (v2.getEps() != null) v1.setEps(v2.getEps());
                    if (v2.getDividendAmount() != null) v1.setDividendAmount(v2.getDividendAmount());
                    if (v2.getDividendFrequency() != null) v1.setDividendFrequency(v2.getDividendFrequency());
                    if (v2.getTradingStatus() != null) v1.setTradingStatus(v2.getTradingStatus());
                    if (v2.getStatusReason() != null) v1.setStatusReason(v2.getStatusReason());
                    if (v2.getHaltStartTime() != null) v1.setHaltStartTime(v2.getHaltStartTime());
                    if (v2.getHaltEndTime() != null) v1.setHaltEndTime(v2.getHaltEndTime());
                    if (v2.getBeta() != null) v1.setBeta(v2.getBeta());
                    if (v2.getOpen() != null) v1.setOpen(v2.getOpen());
                    if (v2.getHigh() != null) v1.setHigh(v2.getHigh());
                    if (v2.getLow() != null) v1.setLow(v2.getLow());
                    if (v2.getPrevClose() != null) v1.setPrevClose(v2.getPrevClose());
                    if (v2.getDayVolume() != null) v1.setDayVolume(v2.getDayVolume());
                    if (v2.getOpenInterest() != null) v1.setOpenInterest(v2.getOpenInterest());
                    if (v2.getFloatShares() != null) v1.setFloatShares(v2.getFloatShares());
                    if (v2.getPreMarketVolume() != null) v1.setPreMarketVolume(v2.getPreMarketVolume());
                    if (v2.getPostMarketVolume() != null) v1.setPostMarketVolume(v2.getPostMarketVolume());
                    return v1;
                });
                symbolsWithProfile.add(upperSym);
                if (symbolsWithProfile.size() >= normalizedSymbols.size()) {
                    snapshotReceived.complete(null);
                }
            });

            // También escuchamos el precio en vivo para calcular el Market Cap (Punto 2 del notebook)
            channel.addMarketDataListener((sym, data) -> {
                if (data.getLastPrice() != null) {
                    lastPricesCache.put(sym.toUpperCase(), data.getLastPrice());
                }
            });

            channel.subscribeFundamentalsBatch(normalizedSymbols);
            // Suscribir a quotes para el precio actual (usando el nuevo método batch)
            dxLinkClient.subscribe(normalizedSymbols); 

            try {
                snapshotReceived.get(8 + (normalizedSymbols.size() / 20), TimeUnit.SECONDS);
            } catch (TimeoutException e) {
                log.warn("Timeout esperando snapshots de fundamentals via dxLink. Recibidos {}/{}", symbolsWithProfile.size(), normalizedSymbols.size());
            }

            // Calculamos el Market Cap dinámico: Shares * Last Price (con fallback a prevClose)
            normalizedSymbols.forEach(sym -> {
                FundamentalData fund = fundamentalsCache.get(sym);
                if (fund != null) {
                    Double lastPrice = lastPricesCache.get(sym);
                    Double basePrice = (lastPrice != null) ? lastPrice : fund.getPrevClose();
                    
                    if (basePrice != null && fund.getSharesOutstanding() != null) {
                        fund.setMarketCap(fund.getSharesOutstanding() * basePrice);
                    }
                }
            });

        } catch (Exception e) {
            log.error("Error en batch fundamentals dxLink", e);
        }

        // Enriquecimiento con Market Metrics REST (para IV, Liquidez y fechas de Earnings)
        try {
            List<Map<String, Object>> metricsList = getMarketMetricsBatch(normalizedSymbols);
            for (Map<String, Object> metric : metricsList) {
                String sym = (String) metric.get("symbol");
                if (sym == null) continue;
                String finalSym = sym.toUpperCase();
                
                FundamentalData fund = fundamentalsCache.computeIfAbsent(finalSym, k -> FundamentalData.builder().symbol(k).build());

                if (metric.get("implied-volatility-index") != null) fund.setImpliedVolatilityIndex(safeConvertToDouble(metric.get("implied-volatility-index")));
                if (metric.get("implied-volatility-index-rank") != null) fund.setImpliedVolatilityRank(safeConvertToDouble(metric.get("implied-volatility-index-rank")));
                if (metric.get("implied-volatility-percentile") != null) fund.setImpliedVolatilityPercentile(safeConvertToDouble(metric.get("implied-volatility-percentile")));
                if (metric.get("liquidity-value") != null) fund.setLiquidity(safeConvertToDouble(metric.get("liquidity-value")));
                if (metric.get("liquidity-rating") != null) fund.setLiquidityRating(safeConvertToInt(metric.get("liquidity-rating")));

                fund.setShortRatio(safeConvertToDouble(metric.get("short-ratio")));
                
                // REST as primary fallback if dxLink failed
                if (fund.getMarketCap() == null) fund.setMarketCap(safeConvertToDouble(metric.get("market-cap")));
                if (fund.getEps() == null) fund.setEps(safeConvertToDouble(metric.get("earnings-per-share")));
                if (fund.getBeta() == null) fund.setBeta(safeConvertToDouble(metric.get("beta")));

                // Dividend Enrichment
                if (fund.getDividendAmount() == null) fund.setDividendAmount(safeConvertToDouble(metric.get("dividend-rate-per-share")));
                
                // Next Earnings Date from nested 'earnings' object
                Object earningsObj = metric.get("earnings");
                Object earningsDateValue = null;
                if (earningsObj instanceof Map) {
                    Map<String, Object> eMap = (Map<String, Object>) earningsObj;
                    earningsDateValue = eMap.get("estimated-report-date");
                    if (earningsDateValue == null) earningsDateValue = eMap.get("actual-report-date");
                }
                if (earningsDateValue == null) earningsDateValue = metric.get("earnings-report-date"); // Legacy/Defensive

                if (earningsDateValue instanceof String) {
                    try {
                        LocalDate earningsDate = LocalDate.parse((String) earningsDateValue);
                        fund.setNextEarningsDate(earningsDate);
                        long days = ChronoUnit.DAYS.between(LocalDate.now(), earningsDate);
                        fund.setDaysUntilEarnings((int) Math.max(0, (int) days));
                    } catch (Exception ignored) {}
                }
            }
        } catch (Exception e) {
            log.warn("No se pudo enriquecer con Market Metrics REST: {}", e.getMessage());
        }

        // Enriquecimiento con Instrument Details en BATCH (para Shares Outstanding y Float)
        try {
            List<Map<String, Object>> instrumentItems = getEquitiesBatch(normalizedSymbols);
            for (Map<String, Object> item : instrumentItems) {
                String sym = (String) item.get("symbol");
                if (sym == null) continue;
                FundamentalData fund = fundamentalsCache.get(sym.toUpperCase());
                if (fund != null) {
                    if (fund.getSharesOutstanding() == null) fund.setSharesOutstanding(safeConvertToLong(item.get("shares-outstanding")));
                    if (fund.getFloatShares() == null) fund.setFloatShares(safeConvertToLong(item.get("free-float")));
                    if (fund.getBeta() == null) fund.setBeta(safeConvertToDouble(item.get("beta")));
                }
            }
        } catch (Exception e) {
            log.warn("No se pudo enriquecer con Instrument Details REST Batch: {}", e.getMessage());
        }

        // Fallback de Cotizaciones OHLC en BATCH (REST) si dxLink falló o está en silencio (Fin de semana)
        try {
            List<Map<String, Object>> ohlcItems = getMarketDataBatch(normalizedSymbols);
            for (Map<String, Object> item : ohlcItems) {
                String sym = (String) item.get("symbol");
                if (sym == null) continue;
                FundamentalData fund = fundamentalsCache.get(sym.toUpperCase());
                if (fund != null) {
                    if (fund.getOpen() == null) fund.setOpen(safeConvertToDouble(item.get("open")));
                    if (fund.getHigh() == null) fund.setHigh(safeConvertToDouble(item.get("high")));
                    if (fund.getLow() == null) fund.setLow(safeConvertToDouble(item.get("low")));
                    if (fund.getPrevClose() == null) fund.setPrevClose(safeConvertToDouble(item.get("prev-close")));
                    // market-cap institucional desde quote REST si el calculado por dxLink falló
                    if (fund.getMarketCap() == null) fund.setMarketCap(safeConvertToDouble(item.get("market-cap")));
                }
            }
        } catch (Exception e) {
            log.warn("No se pudo enriquecer con Market Data REST Batch: {}", e.getMessage());
        }

        return normalizedSymbols.stream()
                .collect(java.util.stream.Collectors.toMap(
                    s -> s,
                    s -> fundamentalsCache.getOrDefault(s, FundamentalData.builder().symbol(s).build())
                ));
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

    /**
     * Motor de Reconciliación de Estado (State Reconciliation Engine).
     * Sincroniza posiciones y órdenes en paralelo para asegurar integridad tras desconexiones.
     */
    public void reconcileAccountState() {
        log.info("🔄 Iniciando Reconciliación de Estado Crítico...");
        
        try {
            CompletableFuture<List<Map<String, Object>>> positionsFuture = CompletableFuture.supplyAsync(
                    tastyTradeClient::getPositions);
            
            CompletableFuture<List<Map<String, Object>>> ordersFuture = CompletableFuture.supplyAsync(
                    tastyTradeClient::getLiveOrders);

            CompletableFuture.allOf(positionsFuture, ordersFuture)
                    .thenAccept(v -> {
                        List<Map<String, Object>> positions = positionsFuture.join();
                        List<Map<String, Object>> orders = ordersFuture.join();
                        
                        log.info("✅ Sincronización Exitosa: {} posiciones y {} órdenes conciliadas.", 
                                positions.size(), orders.size());
                        
                        // Hidratar caché L1 de posiciones y órdenes en memoria
                        positionsCache.clear();
                        positions.forEach(p -> {
                            Object sym = p.get("symbol");
                            if (sym != null) positionsCache.put(sym.toString(), p);
                        });

                        liveOrdersCache.clear();
                        orders.forEach(o -> {
                            Object id = o.get("id");
                            if (id != null) liveOrdersCache.put(id.toString(), o);
                        });
                    }).get(10, TimeUnit.SECONDS);
                    
        } catch (Exception e) {
            log.error("❌ Falló la Reconciliación de Estado. Peligro de trading ciego.", e);
        }
    }

    private void ensureConnected() {
        boolean reconnected = false;

        if (!dxLinkClient.isConnected()) {
            log.info("Reconnecting to DxLink");
            String token = tastyTradeClient.getApiQuoteToken();
            String url = tastyTradeClient.getDxlinkUrl();
            dxLinkClient.connect(url, token);
            reconnected = true;
        }
        if (reconnected && dxLinkClient.getDefaultChannel() != null) {
            dxLinkClient.getDefaultChannel().addFundamentalListener((sym, data) -> {
                String upperSym = sym.toUpperCase();
                fundamentalsCache.merge(upperSym, data, (v1, v2) -> {
                    if (v2.getSharesOutstanding() != null) v1.setSharesOutstanding(v2.getSharesOutstanding());
                    if (v2.getFloatShares() != null) v1.setFloatShares(v2.getFloatShares());
                    if (v2.getEps() != null) v1.setEps(v2.getEps());
                    if (v2.getBeta() != null) v1.setBeta(v2.getBeta());
                    if (v2.getTradingStatus() != null) v1.setTradingStatus(v2.getTradingStatus());
                    return v1;
                });
            });
        }
        
        // Conexión al Account Streamer
        String accessToken = tastyTradeClient.getAccessToken();
        String streamerUrl = tastyTradeClient.getAccountStreamerUrl();
        accountStreamerClient.connect(streamerUrl, accessToken);
        
        if (reconnected) {
            log.info("📡 Reconexión detectada. Ejecutando reconciliación de seguridad...");
            reconcileAccountState();
        }
    }

    private Double safeConvertToDouble(Object val) {
        if (val == null) return null;
        if (val instanceof Number n) {
            double d = n.doubleValue();
            return Double.isFinite(d) ? d : null;
        }
        try {
            double d = Double.parseDouble(val.toString());
            return Double.isFinite(d) ? d : null;
        } catch (Exception e) {
            return null;
        }
    }

    private Long safeConvertToLong(Object val) {
        if (val == null) return null;
        if (val instanceof Number n) {
            long l = n.longValue();
            return Double.isFinite((double) l) ? l : null;
        }
        try {
            return Long.parseLong(val.toString());
        } catch (Exception e) {
            return null;
        }
    }

    private Integer safeConvertToInt(Object val) {
        if (val == null) return null;
        if (val instanceof Number n) {
            int i = n.intValue();
            return Double.isFinite((double) i) ? i : null;
        }
        try {
            return Integer.parseInt(val.toString());
        } catch (Exception e) {
            return null;
        }
    }
}
