package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import java.time.Instant;
import java.time.LocalDate;
import java.time.temporal.ChronoUnit;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.Set;
import java.util.TreeMap;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import java.util.function.Consumer;



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
import com.metradingplat.marketdata.infrastructure.output.external.secedgar.SecEdgarClient;

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
    private final SecEdgarClient secEdgarClient;
    private final java.util.concurrent.ScheduledExecutorService scheduler = java.util.concurrent.Executors.newSingleThreadScheduledExecutor();

    // Trackers para la heuristica de Halt Status (Punto 5)
    private final ConcurrentHashMap<String, Long> lastMarketDataUpdates = new ConcurrentHashMap<>();
    private final ConcurrentHashMap<String, OptionContract> greeksCache = new ConcurrentHashMap<>();

    // Cachés Globales L1 (Arquitectura de Resiliencia)
    private final ConcurrentHashMap<String, FundamentalData> fundamentalsCache = new ConcurrentHashMap<>();
    private final ConcurrentHashMap<String, Double> lastPricesCache = new ConcurrentHashMap<>();
    private final ConcurrentHashMap<String, Map<String, Object>> positionsCache = new ConcurrentHashMap<>();
    private final ConcurrentHashMap<String, Map<String, Object>> liveOrdersCache = new ConcurrentHashMap<>();

    // Cache de quotes REST: solo se usa como fallback si la consulta en vivo a TastyTrade falla
    private final ConcurrentHashMap<String, CachedQuote> restQuoteCache = new ConcurrentHashMap<>();
    private static final long QUOTE_LKG_TTL_MS = 300_000; // 5 minutos para last-known-good
    private static final int MAX_RETRIES = 3;

    // Estado del preload masivo de fundamentales al arrancar (preloadFundamentalsFromRest).
    // completedAt es null mientras el preload no ha terminado; una vez seteado, el preload
    // ya recorrio todo el universo objetivo (no implica 100% de simbolos con datos, TastyTrade
    // puede no tener info de algunos).
    private volatile Instant fundamentalsPreloadStartedAt;
    private volatile Instant fundamentalsPreloadCompletedAt;
    private volatile int fundamentalsPreloadTargetSymbols;
    private volatile int fundamentalsPreloadMarketMetricsLoaded;
    private volatile int fundamentalsPreloadEquitiesLoaded;
    private volatile int fundamentalsPreloadOhlcLoaded;

    public Map<String, Object> getFundamentalsPreloadStatus() {
        Map<String, Object> status = new java.util.LinkedHashMap<>();
        status.put("started", fundamentalsPreloadStartedAt != null);
        status.put("complete", fundamentalsPreloadCompletedAt != null);
        status.put("targetSymbols", fundamentalsPreloadTargetSymbols);
        status.put("marketMetricsLoaded", fundamentalsPreloadMarketMetricsLoaded);
        status.put("equitiesLoaded", fundamentalsPreloadEquitiesLoaded);
        status.put("ohlcLoaded", fundamentalsPreloadOhlcLoaded);
        status.put("cachedSymbols", fundamentalsCache.size());
        status.put("startedAt", fundamentalsPreloadStartedAt);
        status.put("completedAt", fundamentalsPreloadCompletedAt);
        return status;
    }

    private record CachedQuote(double price, long timestamp, boolean stale) {}

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

        dxLinkClient.setOnGreeks((symbol, greeks) -> {
            Double iv = greeks.getImpliedVolatility();
            if (iv != null && iv > 1.0) { // Señal de alta volatilidad (>100% IV)
                log.warn("🔥 [ALPHA SIGNAL] Volatilidad Extrema en {}: IV={}% | Delta={}", 
                        symbol, Math.round(iv * 100), greeks.getDelta());
            }
            
            greeksCache.put(symbol, greeks);
        });

        log.info("DxLink initialized. Auto-subscribe and REST preload will start in 5s...");

        // Ancladas antes de la apertura regular (9:30am ET) -- antes eran 8/9/10am,
        // lo que dejaba el refresco diario a medio terminar (o sin empezar) cuando
        // ya habia arrancado la sesion. Separadas por una hora para no golpear
        // TastyTrade/FINRA/SEC al mismo tiempo.
        scheduler.scheduleAtFixedRate(this::refreshMarketMetrics,
                millisUntilNextHour(2), 4 * 3600_000, TimeUnit.MILLISECONDS);
        scheduler.scheduleAtFixedRate(this::checkFinraForUpdate,
                millisUntilNextHour(3), 4 * 3600_000, TimeUnit.MILLISECONDS);
        scheduler.scheduleAtFixedRate(this::refreshSharesOutstandingFromSecEdgar,
                millisUntilNextHour(4), 24 * 3600_000, TimeUnit.MILLISECONDS);
        scheduler.scheduleAtFixedRate(this::cleanupStaleCandles,
                5, 5, TimeUnit.MINUTES);
        scheduler.scheduleAtFixedRate(this::cleanupStaleRestQuotes,
                5, 5, TimeUnit.MINUTES);
        scheduler.scheduleAtFixedRate(this::refreshOhlcData,
                5, 5, TimeUnit.MINUTES);
        scheduler.scheduleAtFixedRate(this::fillMissingSharesOutstandingFromSecEdgar,
                5, 5, TimeUnit.MINUTES);

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
        log.info("Preloading fundamentals for all US equity markets: {}", ALL_US_MARKETS);
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
            log.info("Starting fundamentals preload for {} symbols (streaming quotes on demand)", distinctSymbols.size());
            CompletableFuture.runAsync(() -> preloadFundamentalsFromRest(distinctSymbols));
        }
    }

    private void preloadFundamentalsFromRest(List<String> symbols) {
        log.info("Bulk preloading REST fundamentals for {} symbols", symbols.size());
        fundamentalsPreloadStartedAt = Instant.now();
        fundamentalsPreloadTargetSymbols = symbols.size();
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
                    Double betaVal = safeConvertToDouble(m.get("beta"));
                    if (betaVal != null && betaVal != 0.0) fund.setBeta(betaVal);
                    Double epsVal = safeConvertToDouble(m.get("earnings-per-share"));
                    if (epsVal != null && epsVal != 0.0) fund.setEps(epsVal);
                    Double mcVal = safeConvertToDouble(m.get("market-cap"));
                    if (mcVal != null && mcVal > 0) fund.setMarketCap(mcVal);
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
                    boolean isEtf = Boolean.TRUE.equals(eq.get("is-etf"));
                    fund.setIsEtf(isEtf);
                    fund.setSecurityType(classifySecurityType(sym.toUpperCase(), isEtf, (String) eq.get("description")));
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
                    if (fund.getHigh() == null) fund.setHigh(safeConvertToDouble(item.get("day-high-price")));
                    if (fund.getLow() == null) fund.setLow(safeConvertToDouble(item.get("day-low-price")));
                    if (fund.getPrevClose() == null) fund.setPrevClose(safeConvertToDouble(item.get("prev-close")));
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

        fundamentalsPreloadMarketMetricsLoaded = loaded;
        fundamentalsPreloadEquitiesLoaded = equityLoaded;
        fundamentalsPreloadOhlcLoaded = ohlcLoaded;
        fundamentalsPreloadCompletedAt = Instant.now();
        log.info("Fundamentals preload complete: target={}, marketMetrics={}, equities={}, ohlc={}",
                symbols.size(), loaded, equityLoaded, ohlcLoaded);

        CompletableFuture.runAsync(this::updateShortInterestFromFinra);
        CompletableFuture.runAsync(this::refreshSharesOutstandingFromSecEdgar);
        CompletableFuture.runAsync(() -> {
            List<String> stillMissing = new ArrayList<>();
            for (String sym : symbols) {
                FundamentalData fund = fundamentalsCache.get(sym.toUpperCase());
                if (fund == null || fund.getBeta() == null || fund.getSharesOutstanding() == null) {
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

    private static final int SUBSCRIBE_CHUNK_SIZE = 100;
    private static long millisUntilNextHour(int targetHour) {
        java.time.ZonedDateTime now = java.time.ZonedDateTime.now(java.time.ZoneId.of("America/New_York"));
        java.time.ZonedDateTime next = now.withMinute(5).withSecond(0).withNano(0);
        if (next.getHour() >= targetHour) next = next.plusDays(1);
        next = next.withHour(targetHour).withMinute(0);
        return java.time.Duration.between(now, next).toMillis();
    }

    private volatile String lastFinraSettlement;

    private void refreshOhlcData() {
        List<String> allSymbols = new ArrayList<>(fundamentalsCache.keySet());
        if (allSymbols.isEmpty()) return;
        int updated = 0;
        for (int i = 0; i < allSymbols.size(); i += 100) {
            int end = Math.min(i + 100, allSymbols.size());
            List<String> chunk = allSymbols.subList(i, end);
            try {
                List<Map<String, Object>> ohlc = tastyTradeClient.getMarketDataBatch(chunk);
                for (Map<String, Object> item : ohlc) {
                    String sym = (String) item.get("symbol");
                    if (sym == null) continue;
                    FundamentalData fund = fundamentalsCache.get(sym.toUpperCase());
                    if (fund == null) continue;
                    Double high = safeConvertToDouble(item.get("day-high-price"));
                    Double low = safeConvertToDouble(item.get("day-low-price"));
                    Long volume = safeConvertToLong(item.get("volume"));
                    Double open = safeConvertToDouble(item.get("open"));
                    Double prevClose = safeConvertToDouble(item.get("prev-close"));
                    if (high != null) fund.setHigh(high);
                    if (low != null) fund.setLow(low);
                    if (volume != null) fund.setDayVolume(volume);
                    if (open != null) fund.setOpen(open);
                    if (prevClose != null) fund.setPrevClose(prevClose);

                    if (item.get("is-trading-halted") != null) {
                        fund.setTradingStatus(Boolean.TRUE.equals(item.get("is-trading-halted")) ? "HALTED" : "ACTIVE");
                    }
                    Long haltStart = safeConvertToLong(item.get("halt-start-time"));
                    Long haltEnd = safeConvertToLong(item.get("halt-end-time"));
                    if (haltStart != null) fund.setHaltStartTime(haltStart);
                    if (haltEnd != null) fund.setHaltEndTime(haltEnd);

                    // Fuente principal de marketCap, recalculado cada 5 min con el precio
                    // mas fresco disponible (para simbolos con suscripcion de velas activa,
                    // recomputeMarketCapFromLivePrice ya lo mantiene al dia por bar/tick en
                    // vivo). El market-cap real de TastyTrade solo se usa como semilla en el
                    // preload inicial -- refreshMarketMetrics ya no lo vuelve a escribir, para
                    // que no compitan dos fuentes con cadencias distintas por el mismo campo.
                    if (fund.getSharesOutstanding() != null && fund.getSharesOutstanding() > 0) {
                        Double price = prevClose != null ? prevClose : (open != null ? open : fund.getPrevClose());
                        if (price == null) price = fund.getOpen();
                        if (price != null && price > 0) {
                            fund.setMarketCap(fund.getSharesOutstanding() * price);
                        }
                    }
                    updated++;
                }
            } catch (Exception e) {
                log.warn("OHLC refresh chunk at {} failed: {}", i, e.getMessage());
            }
        }
        log.info("OHLC refresh: {} symbols updated (live high/low/volume)", updated);
    }

    private void cleanupStaleRestQuotes() {
        long cutoff = System.currentTimeMillis() - QUOTE_LKG_TTL_MS;
        int before = restQuoteCache.size();
        restQuoteCache.values().removeIf(q -> q.timestamp() < cutoff);
        log.info("REST quote cache cleanup: {} -> {} entries", before, restQuoteCache.size());
    }

    private void refreshMarketMetrics() {
        log.info("Scheduled market-metrics refresh starting...");
        try {
            List<String> allSymbols = new ArrayList<>(fundamentalsCache.keySet());
            if (allSymbols.isEmpty()) return;
            int updated = 0;
            for (int i = 0; i < allSymbols.size(); i += 250) {
                int end = Math.min(i + 250, allSymbols.size());
                List<String> chunk = allSymbols.subList(i, end);
                List<Map<String, Object>> metrics = tastyTradeClient.getMarketMetricsBatch(chunk);
                for (Map<String, Object> m : metrics) {
                    String sym = (String) m.get("symbol");
                    if (sym == null) continue;
                    FundamentalData fund = fundamentalsCache.computeIfAbsent(sym.toUpperCase(),
                            k -> FundamentalData.builder().symbol(k).build());
                    fund.setImpliedVolatilityIndex(safeConvertToDouble(m.get("implied-volatility-index")));
                    fund.setImpliedVolatilityRank(safeConvertToDouble(m.get("implied-volatility-index-rank")));
                    fund.setImpliedVolatilityPercentile(safeConvertToDouble(m.get("implied-volatility-percentile")));
                    fund.setLiquidity(safeConvertToDouble(m.get("liquidity-value")));
                    fund.setLiquidityRating(safeConvertToInt(m.get("liquidity-rating")));
                    fund.setBorrowRate(safeConvertToDouble(m.get("borrow-rate")));
                    fund.setLendability((String) m.get("lendability"));

                    // Estos campos antes solo se llenaban una vez (en el preload inicial) y
                    // nunca se refrescaban en este job periodico. TastyTrade ya los trae en
                    // esta misma llamada, asi que actualizarlos aqui no agrega ninguna
                    // peticion REST nueva. marketCap NO se toca aqui: refreshOhlcData ya lo
                    // recalcula cada 5 min (y en vivo por bar/tick si hay suscripcion activa
                    // de velas), y es la fuente principal.
                    Double betaVal = safeConvertToDouble(m.get("beta"));
                    if (betaVal != null && betaVal != 0.0) fund.setBeta(betaVal);
                    Double eps = safeConvertToDouble(m.get("earnings-per-share"));
                    if (eps != null) fund.setEps(eps);
                    Double dividendAmount = safeConvertToDouble(m.get("dividend-rate-per-share"));
                    if (dividendAmount != null) fund.setDividendAmount(dividendAmount);
                    // TastyTrade solo tiene su propio short-ratio como estimado; FINRA es la
                    // fuente autoritativa (checkFinraForUpdate), asi que no se sobreescribe.
                    if (fund.getShortRatio() == null) {
                        fund.setShortRatio(safeConvertToDouble(m.get("short-ratio")));
                    }

                    Object earningsObj = m.get("earnings");
                    String earningsDateStr = null;
                    if (earningsObj instanceof Map<?, ?> earningsMap) {
                        Object raw = earningsMap.get("expected-report-date");
                        if (raw == null) raw = earningsMap.get("estimated-report-date");
                        if (raw instanceof String s) earningsDateStr = s;
                    }
                    if (earningsDateStr != null) {
                        try {
                            LocalDate earningsDate = LocalDate.parse(earningsDateStr);
                            fund.setNextEarningsDate(earningsDate);
                            long days = ChronoUnit.DAYS.between(LocalDate.now(), earningsDate);
                            fund.setDaysUntilEarnings((int) Math.max(0, days));
                        } catch (Exception ignored) {}
                    }
                    updated++;
                }
            }
            log.info("Market-metrics refresh: {} symbols updated", updated);
        } catch (Exception e) {
            log.warn("Market-metrics refresh failed: {}", e.getMessage());
        }
    }

    /**
     * Gap-filler independiente, cada 5 min (igual que refreshOhlcData, del cual
     * depende marketCap): revisa SOLO los simbolos que aun sigan sin sharesOutstanding
     * contra el archivo de SEC EDGAR ya cacheado en disco (secEdgarClient.ensureCachedZip()
     * reutiliza el de hoy si existe, no descarga de nuevo). TastyTrade nunca trae
     * sharesOutstanding, asi que sin esto el campo se quedaria null para siempre en
     * los simbolos que el refresco diario no alcanzo a resolver. El refresco diario
     * completo de todos los simbolos sigue siendo refreshSharesOutstandingFromSecEdgar();
     * esto es barato porque no toca red, solo CPU local sobre el archivo ya en disco.
     */
    private void fillMissingSharesOutstandingFromSecEdgar() {
        List<String> stillMissing = new ArrayList<>();
        for (var entry : fundamentalsCache.entrySet()) {
            if (entry.getValue().getSharesOutstanding() == null) stillMissing.add(entry.getKey());
        }
        if (stillMissing.isEmpty()) return;
        log.info("SEC EDGAR gap-fill: {} symbols still missing sharesOutstanding", stillMissing.size());
        try {
            Map<String, Long> shares = secEdgarClient.fetchSharesOutstanding(stillMissing);
            for (var entry : shares.entrySet()) {
                FundamentalData fund = fundamentalsCache.get(entry.getKey());
                if (fund == null) continue;
                fund.setSharesOutstanding(entry.getValue());
                fund.setFloatShares(Math.round(entry.getValue() * 0.90));
            }
            log.info("SEC EDGAR gap-fill: filled sharesOutstanding for {} symbols", shares.size());
        } catch (Exception e) {
            log.warn("SEC EDGAR gap-fill failed: {}", e.getMessage());
        }
    }

    private EnumTimeframe parseTimeframeFromLabel(String label) {
        if (label == null) return null;
        return switch (label) {
            case "1m" -> EnumTimeframe.M1;
            case "5m" -> EnumTimeframe.M5;
            case "15m" -> EnumTimeframe.M15;
            case "30m" -> EnumTimeframe.M30;
            case "1h" -> EnumTimeframe.H1;
            case "1d" -> EnumTimeframe.D1;
            case "1w" -> EnumTimeframe.W1;
            case "1mo" -> EnumTimeframe.MO1;
            default -> null;
        };
    }

    private void cleanupStaleCandles() {
        long now = System.currentTimeMillis();
        List<String> toRemove = new ArrayList<>();
        log.info("Candle cleanup: checking {} cached symbols", candleLastAccess.size());
        for (var entry : candleLastAccess.entrySet()) {
            String key = entry.getKey();
            long lastAccess = entry.getValue();
            String[] parts = key.split("\\|");
            if (parts.length != 2) continue;
            String tfName = parts[1];
            EnumTimeframe tf;
            try { tf = EnumTimeframe.valueOf(tfName); }
            catch (IllegalArgumentException e) { continue; }
            long maxIdle = tf.getDuration().multipliedBy(2).toMillis();
            if (maxIdle < 300_000) maxIdle = 300_000;
            if (now - lastAccess > maxIdle) {
                toRemove.add(key);
                log.info("Candle auto-unsubscribe: {} (idle {}s, max {}s, timeframe={})",
                        key, (now - lastAccess) / 1000, maxIdle / 1000, tfName);
            }
        }
        if (!toRemove.isEmpty()) {
            log.info("Auto-unsubscribing {} stale candle symbols", toRemove.size());
            for (String key : toRemove) {
                candleCache.remove(key);
                candleLastAccess.remove(key);
                subscribedCandleSymbols.remove(key);
            }
        }
    }

    private void refreshSharesOutstandingFromSecEdgar() {
        // Re-verifica todos los simbolos cacheados cada dia, no solo los que aun
        // estan en null: sharesOutstanding puede cambiar (recompras, emisiones) y
        // uno que ya tenga valor se quedaria congelado para siempre si solo
        // llenamos huecos. Esto no cuesta nada extra: el companyfacts.zip completo
        // de SEC se descarga una vez al dia sin importar cuantos simbolos pidamos,
        // asi que filtrar a "solo los que faltan" solo reducia cuantos leiamos de
        // un archivo que ya estaba en disco de todas formas.
        List<String> allCached = new ArrayList<>(fundamentalsCache.keySet());
        if (!allCached.isEmpty()) {
            log.info("SEC EDGAR refresh: re-checking sharesOutstanding for {} symbols", allCached.size());
            try {
                Map<String, Long> shares = secEdgarClient.fetchSharesOutstanding(allCached);
                for (var entry : shares.entrySet()) {
                    FundamentalData fund = fundamentalsCache.get(entry.getKey());
                    if (fund == null) continue;
                    fund.setSharesOutstanding(entry.getValue());
                    // floatShares no tiene fuente propia -- siempre es 90% de
                    // sharesOutstanding, asi que se recalcula junto con el,
                    // sin importar si ya tenia un valor de un dia anterior.
                    fund.setFloatShares(Math.round(entry.getValue() * 0.90));
                }
                log.info("SEC EDGAR refresh: updated sharesOutstanding for {} symbols", shares.size());
            } catch (Exception e) {
                log.warn("SEC EDGAR refresh failed: {}", e.getMessage());
            }
        }
        logSharesOutstandingCoverage();
    }

    private String classifySecurityType(String symbol, boolean isEtf, String description) {
        if (symbol != null && symbol.contains("TEST")) return "TEST_SYMBOL";
        String d = description != null ? description.trim().toLowerCase() : "";
        if (d.contains("tick pilot") || d.contains("symbology tst")) return "TEST_SYMBOL";
        if (isEtf || d.contains("etf")) return "ETF";
        if (d.matches(".*\\bnotes? due\\b.*") || d.matches(".*\\b(senior|subordinated) notes\\b.*")
                || d.contains("mortgage bonds")) return "BOND";
        if (d.contains("preferred stock") || d.contains("preferred units") || d.contains("preferred shares")
                || d.contains("depositary shares")) return "PREFERRED";
        if (symbol != null && symbol.endsWith("/WS")) return "WARRANT";
        if (d.matches(".*\\bwarrants?\\b.*")) return "WARRANT";
        if (symbol != null && symbol.endsWith("/U")) return "UNIT";
        if (d.matches(".*\\buts?\\b.*") || d.matches(".*\\bunits?\\b.*")) return "UNIT";
        if (d.matches(".*\\brights?\\b.*")) return "RIGHTS";
        if (description == null && symbol != null && symbol.matches("^[A-Z]{2,6}P[A-Z]{1,2}$")) return "PREFERRED";
        return "EQUITY";
    }

    private void logSharesOutstandingCoverage() {
        Map<String, int[]> byType = new TreeMap<>();
        for (FundamentalData fund : fundamentalsCache.values()) {
            String type = fund.getSecurityType() != null ? fund.getSecurityType() : "UNKNOWN";
            int[] counts = byType.computeIfAbsent(type, k -> new int[2]);
            counts[0]++;
            if (fund.getSharesOutstanding() != null) counts[1]++;
        }
        StringBuilder sb = new StringBuilder("SharesOutstanding coverage by type:");
        for (var entry : byType.entrySet()) {
            sb.append(String.format(" %s=%d/%d", entry.getKey(), entry.getValue()[1], entry.getValue()[0]));
        }
        log.info(sb.toString());
    }

    private void checkFinraForUpdate() {
        log.info("Checking FINRA for new short interest data...");
        try {
            Map<String, FinraClient.ShortInterestRecord> finraData = finraClient.downloadLatest();
            if (finraData.isEmpty()) return;

            String newSettlement = finraData.values().iterator().next().settlementDate;
            if (newSettlement.equals(lastFinraSettlement)) {
                log.debug("FINRA data unchanged (settlement {})", lastFinraSettlement);
                return;
            }

            int updated = 0;
            for (var entry : finraData.entrySet()) {
                String sym = entry.getKey();
                FinraClient.ShortInterestRecord rec = entry.getValue();
                FundamentalData fund = fundamentalsCache.get(sym);
                if (fund == null) continue;
                fund.setShortRatio(rec.daysToCover > 0 && rec.daysToCover < 999 ? rec.daysToCover : null);
                fund.setDayVolume(rec.avgDailyVolume > 0 ? rec.avgDailyVolume : null);
                if (fund.getFloatShares() != null && fund.getFloatShares() > 0 && rec.sharesShorted > 0) {
                    double shortPct = (double) rec.sharesShorted / fund.getFloatShares() * 100.0;
                    fund.setShortInterest(Math.round(shortPct * 100.0) / 100.0);
                }
                updated++;
            }
            lastFinraSettlement = newSettlement;
            log.info("FINRA short interest refreshed: {} symbols (settlement {})", updated, newSettlement);
        } catch (Exception e) {
            log.warn("FINRA check failed: {}", e.getMessage());
        }
    }

    private void updateShortInterestFromFinra() {
        log.info("Downloading FINRA short interest data...");
        try {
            Map<String, FinraClient.ShortInterestRecord> finraData = finraClient.downloadLatest();
            if (finraData.isEmpty()) return;

            int updated = 0;
            for (var entry : finraData.entrySet()) {
                String sym = entry.getKey();
                FinraClient.ShortInterestRecord rec = entry.getValue();
                FundamentalData fund = fundamentalsCache.get(sym);
                if (fund == null) continue;

                fund.setShortRatio(rec.daysToCover > 0 && rec.daysToCover < 999 ? rec.daysToCover : null);
                fund.setDayVolume(rec.avgDailyVolume > 0 ? rec.avgDailyVolume : null);

                if (updated < 3) {
                    log.info("FINRA sample {}: sharesShorted={}, avgVol={}, daysToCover={}, floatShares={}",
                            sym, rec.sharesShorted, rec.avgDailyVolume, rec.daysToCover, fund.getFloatShares());
                }

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

    private static final long SUBSCRIBE_CHUNK_DELAY_MS = 50;

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
            if (price != null && price > 0) {
                result.put(upper, price);
            }
        }
        return result;
    }

    public Map<String, Double> getRestQuotes(List<String> symbols) {
        Map<String, Double> result = new ConcurrentHashMap<>();
        long now = System.currentTimeMillis();

        List<Map<String, Object>> items = fetchWithRetry(symbols);
        Set<String> fetched = ConcurrentHashMap.newKeySet();
        for (Map<String, Object> item : items) {
            String sym = (String) item.get("symbol");
            Object last = item.get("last");
            if (sym != null && last != null) {
                double price = safeConvertToDouble(last);
                if (price > 0) {
                    String upper = sym.toUpperCase();
                    result.put(upper, price);
                    restQuoteCache.put(upper, new CachedQuote(price, now, false));
                    fetched.add(upper);
                }
            }
        }

        for (String sym : symbols) {
            String upper = sym.toUpperCase();
            if (!fetched.contains(upper)) {
                CachedQuote lkg = restQuoteCache.get(upper);
                if (lkg != null && (now - lkg.timestamp) < QUOTE_LKG_TTL_MS) {
                    result.put(upper, lkg.price);
                }
            }
        }

        return result;
    }

    private List<Map<String, Object>> fetchWithRetry(List<String> symbols) {
        for (int attempt = 1; attempt <= MAX_RETRIES; attempt++) {
            try {
                List<Map<String, Object>> items = tastyTradeClient.getMarketDataBatch(symbols);
                if (!items.isEmpty()) return items;
                if (attempt < MAX_RETRIES) {
                    log.warn("REST quotes empty, retry {}/{}", attempt, MAX_RETRIES);
                    Thread.sleep(200 * attempt);
                }
            } catch (Exception e) {
                if (attempt < MAX_RETRIES) {
                    log.warn("REST quotes failed (attempt {}): {}", attempt, e.getMessage());
                    try { Thread.sleep(300 * attempt); } catch (InterruptedException ie) { Thread.currentThread().interrupt(); break; }
                } else {
                    log.error("REST quotes failed after {} retries: {}", MAX_RETRIES, e.getMessage());
                }
            }
        }
        return List.of();
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
                    Double subShortRatio = safeConvertToDouble(m.get("short-ratio"));
                    if (subShortRatio != null) fund.setShortRatio(subShortRatio);
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
     * Obtiene candles historicos de un solo simbolo. Delega al metodo batch.
     */
    public List<Candle> getCandles(String symbol, EnumTimeframe timeframe) {
        log.debug("Fetching candles for {} {}", symbol, timeframe);
        Map<String, List<Candle>> result = getCandlesBatch(List.of(symbol), timeframe, 700);
        return result.getOrDefault(symbol, List.of());
    }

    private DxLinkClient.DxLinkChannel candleChannel;
    private final ConcurrentHashMap<String, List<Candle>> candleCache = new ConcurrentHashMap<>();
    private final ConcurrentHashMap<String, Long> candleLastAccess = new ConcurrentHashMap<>();
    private final Set<String> subscribedCandleSymbols = ConcurrentHashMap.newKeySet();
    private final ConcurrentHashMap<String, List<Consumer<Candle>>> candleLiveListeners = new ConcurrentHashMap<>();

    /**
     * Registra un listener para velas en vivo (usado por CandleWebSocketHandler).
     * Se apoya en el mismo canal persistente/cache que getCandlesBatch, así que
     * comparte el idle-cleanup existente en cleanupStaleCandles().
     */
    public void addCandleLiveListener(String symbol, EnumTimeframe timeframe, Consumer<Candle> listener) {
        ensureCandleChannel();
        subscribeToCandles(List.of(symbol), timeframe);
        String key = symbol.toUpperCase() + "|" + timeframe.name();
        candleLastAccess.put(key, System.currentTimeMillis());
        candleLiveListeners.computeIfAbsent(key, k -> new CopyOnWriteArrayList<>()).add(listener);
    }

    public void removeCandleLiveListener(String symbol, EnumTimeframe timeframe, Consumer<Candle> listener) {
        String key = symbol.toUpperCase() + "|" + timeframe.name();
        List<Consumer<Candle>> listeners = candleLiveListeners.get(key);
        if (listeners == null) return;
        listeners.remove(listener);
        if (listeners.isEmpty()) candleLiveListeners.remove(key);
    }

    public Map<String, List<Candle>> getCandlesBatch(List<String> symbols, EnumTimeframe timeframe, int bars) {
        Map<String, List<Candle>> result = new ConcurrentHashMap<>();
        List<String> missing = new ArrayList<>();

        for (String sym : symbols) {
            String key = sym.toUpperCase() + "|" + timeframe.name();
            candleLastAccess.put(key, System.currentTimeMillis());
            List<Candle> cached = candleCache.get(key);
            if (cached != null) {
                if (!cached.isEmpty()) {
                    List<Candle> sorted = cached.stream()
                            .filter(c -> c.getTimestamp() != null)
                            .sorted(java.util.Comparator.comparing(Candle::getTimestamp))
                            .collect(java.util.stream.Collectors.toList());
                    if (sorted.size() > bars) sorted = sorted.subList(sorted.size() - bars, sorted.size());
                    result.put(sym.toUpperCase(), sorted);
                }
            } else {
                missing.add(sym);
            }
        }

        if (!missing.isEmpty()) {
            log.info("Candle cache miss for {}/{} symbols, fetching via persistent channel", missing.size(), symbols.size());
            ensureCandleChannel();
            if (candleChannel != null) {
                subscribeToCandles(missing, timeframe);
                Map<String, List<Candle>> fetched = fetchCandlesFromDxLink(missing, timeframe, 700);
                for (var entry : fetched.entrySet()) {
                    String key = entry.getKey() + "|" + timeframe.name();
                    candleCache.put(key, entry.getValue() != null ? entry.getValue() : List.of());
                }
                for (String sym : missing) {
                    String key = sym.toUpperCase() + "|" + timeframe.name();
                    candleCache.putIfAbsent(key, List.of());
                }
                for (String sym : missing) {
                    String key = sym.toUpperCase() + "|" + timeframe.name();
                    List<Candle> fromCache = candleCache.get(key);
                    if (fromCache != null && !fromCache.isEmpty()) {
                        List<Candle> sorted = fromCache.stream()
                                .filter(c -> c.getTimestamp() != null)
                                .sorted(java.util.Comparator.comparing(Candle::getTimestamp))
                                .collect(java.util.stream.Collectors.toList());
                        if (sorted.size() > bars) sorted = sorted.subList(sorted.size() - bars, sorted.size());
                        result.put(sym.toUpperCase(), sorted);
                    }
                }
            }
        }
        return result;
    }

    private synchronized void ensureCandleChannel() {
        if (candleChannel != null && candleChannel.isReady()) return;
        try {
            candleChannel = dxLinkClient.openNewChannel().get(10, TimeUnit.SECONDS);
            candleChannel.addCandleListener((symbol, candle, isSnapshotComplete) -> {
                String cleanSymbol = symbol.replaceAll("\\{=.*\\}", "");
                candle.setSymbol(cleanSymbol);
                java.util.regex.Matcher tfMatcher = java.util.regex.Pattern.compile("\\{=(.*?)\\}").matcher(symbol);
                String tfLabel = tfMatcher.find() ? tfMatcher.group(1) : "UNKNOWN";
                EnumTimeframe tf = parseTimeframeFromLabel(tfLabel);
                if (tf != null) candle.setTimeframe(tf);
                String key = cleanSymbol.toUpperCase() + "|" + (tf != null ? tf.name() : "UNKNOWN");
                List<Candle> list = candleCache.computeIfAbsent(key, k -> new java.util.ArrayList<>());
                java.util.Optional<Candle> existing = list.stream()
                        .filter(c -> c.getTimestamp().equals(candle.getTimestamp())).findFirst();
                Candle merged;
                if (existing.isPresent()) {
                    merged = existing.get();
                    if (candle.getHigh() != null && (merged.getHigh() == null || candle.getHigh() > merged.getHigh())) merged.setHigh(candle.getHigh());
                    if (candle.getLow() != null && (merged.getLow() == null || candle.getLow() < merged.getLow())) merged.setLow(candle.getLow());
                    if (candle.getClose() != null) merged.setClose(candle.getClose());
                    if (candle.getVolume() != null) merged.setVolume(candle.getVolume());
                } else {
                    if (list.size() >= 2000) list.remove(0);
                    list.add(candle);
                    merged = candle;
                }
                candleLastAccess.put(key, System.currentTimeMillis());
                List<Consumer<Candle>> liveListeners = candleLiveListeners.get(key);
                if (liveListeners != null) {
                    for (Consumer<Candle> liveListener : liveListeners) liveListener.accept(merged);
                }
                recomputeMarketCapFromLivePrice(cleanSymbol, merged.getClose());
            });
            log.info("Persistent candle channel opened");
        } catch (Exception e) {
            log.error("Failed to open candle channel", e);
            candleChannel = null;
        }
    }

    /**
     * Recalcula marketCap (sharesOutstanding x precio) cada vez que llega una vela
     * en vivo para un simbolo con suscripcion activa en el canal persistente de
     * velas -- ya sea por un chart del frontend o porque un escaner la sigue
     * pidiendo. En cuanto nadie la pide, cleanupStaleCandles() la da de baja y
     * marketCap vuelve a depender solo del refresco REST de 5 min (refreshOhlcData).
     */
    private void recomputeMarketCapFromLivePrice(String symbol, Double price) {
        if (price == null || price <= 0) return;
        FundamentalData fund = fundamentalsCache.get(symbol.toUpperCase());
        if (fund == null || fund.getSharesOutstanding() == null || fund.getSharesOutstanding() <= 0) return;
        fund.setMarketCap(fund.getSharesOutstanding() * price);
    }

    private void subscribeToCandles(List<String> symbols, EnumTimeframe timeframe) {
        String label = timeframe.getLabel();
        String type = label.substring(label.length() - 1);
        String period = label.substring(0, label.length() - 1);
        List<String> newSymbols = new ArrayList<>();
        for (String s : symbols) {
            if (subscribedCandleSymbols.add(s.toUpperCase() + "|" + timeframe.name())) {
                newSymbols.add(s);
            }
        }
        if (newSymbols.isEmpty()) return;
        log.info("Subscribing {} candle symbols to persistent channel", newSymbols.size());
        for (int i = 0; i < newSymbols.size(); i += 200) {
            int end = Math.min(i + 200, newSymbols.size());
            List<Map<String, Object>> items = new java.util.ArrayList<>();
            for (String s : newSymbols.subList(i, end)) {
                Map<String, Object> item = new java.util.HashMap<>();
                item.put("symbol", String.format("%s{=%s%s}", s, period, type));
                item.put("type", "Candle");
                items.add(item);
            }
            Instant fromTime = Instant.now().minus(timeframe.getDuration().multipliedBy(1400L));
            candleChannel.subscribeCandlesHistory(items, fromTime.toEpochMilli());
        }
    }

    /**
     * dxLink has no reliable per-record "batch finished" signal for historical candles
     * (verified: the wire format doesn't carry one at all with the currently-requested
     * CANDLE_FIELDS). Completion is instead detected by a quiet period: once candles
     * start arriving, wait until QUIET_PERIOD_MS pass with no new one, capped by an
     * overall hard timeout. This is also safer than assuming a single message contains
     * the whole batch, in case dxLink ever splits a large history across more than one.
     *
     * Se probo subir esto a 5000ms para descartar que el backpressure documentado de
     * dxLink (el servidor baja la velocidad de entrega segun consumo del cliente) causara
     * cortes prematuros en lotes grandes -- 1000 simbolos en frio dieron exactamente los
     * mismos 100 simbolos/478 velas con 1000ms y con 5000ms (mas eventos crudos duplicados,
     * cero simbolos nuevos), descartando esa hipotesis. El techo es real del lado de dxLink,
     * no un heuristico nuestro demasiado impaciente -- no vale la pena la latencia extra.
     */
    private static final long CANDLE_QUIET_PERIOD_MS = 1000;

    /**
     * EXPERIMENTO: por encima de este tamano, el histórico se reparte entre varios
     * canales DxLink en paralelo en vez de uno solo, para averiguar si el techo de
     * ~100 simbolos que vimos en pruebas (1000 simbolos en frio -> siempre 100, sin
     * importar el quiet-period) es un limite por canal o por conexion completa.
     */
    private static final int CANDLE_CHANNEL_SPLIT_THRESHOLD = 150;
    private static final int CANDLE_MAX_CHANNELS = 10;

    private Map<String, List<Candle>> fetchCandlesFromDxLink(List<String> symbols, EnumTimeframe timeframe, int bars) {
        int channelCount = symbols.size() > CANDLE_CHANNEL_SPLIT_THRESHOLD
                ? Math.min(CANDLE_MAX_CHANNELS, (symbols.size() + 99) / 100)
                : 1;
        log.info("Fetching candles via DxLink: {} symbols, timeframe={}, channels={}", symbols.size(), timeframe, channelCount);
        Instant now = Instant.now();
        Instant fromTime = now.minus(timeframe.getDuration().multipliedBy((long)(bars * 1.5)));
        // Floor the lookback at a week: for short timeframes (M1..H1) the natural
        // bars*1.5 window can be as little as ~17h, which comes up empty outside
        // market hours (weekends, holidays) since there's simply no candle within
        // that window yet — not a bug, just too short a net. A week comfortably
        // spans any weekend/holiday gap back to the last real trading session.
        Instant minFromTime = now.minus(7, java.time.temporal.ChronoUnit.DAYS);
        if (fromTime.isAfter(minFromTime)) fromTime = minFromTime;
        // 270 days was cutting D1's natural ~3-year (bars*1.5) window down to ~9
        // months, and gutting W1/MO1 (whose natural window is decades) to a
        // handful of bars. 5 years is still a sane upper bound — the quiet-period
        // wait in the loop below is what actually bounds request time, not this.
        if (java.time.Duration.between(fromTime, now).toDays() > 1825)
            fromTime = now.minus(1825, java.time.temporal.ChronoUnit.DAYS);
        String label = timeframe.getLabel();
        String type = label.substring(label.length() - 1);
        String period = label.substring(0, label.length() - 1);
        ensureConnected();
        Map<String, List<Candle>> resultado = new ConcurrentHashMap<>();
        java.util.concurrent.atomic.AtomicLong lastEventAt = new java.util.concurrent.atomic.AtomicLong(0);
        List<DxLinkClient.DxLinkChannel> openedChannels = new ArrayList<>();
        try {
            DxLinkClient.CandleCallback sharedListener = (symbol, candle, isSnapshotComplete) -> {
                String cleanSymbol = symbol.replaceAll("\\{=.*\\}", "");
                candle.setSymbol(cleanSymbol);
                candle.setTimeframe(timeframe);
                List<Candle> list = resultado.computeIfAbsent(cleanSymbol, k -> new java.util.ArrayList<>());
                java.util.Optional<Candle> ex = list.stream()
                        .filter(c -> c.getTimestamp().equals(candle.getTimestamp())).findFirst();
                if (ex.isPresent()) {
                    Candle c = ex.get();
                    if (candle.getHigh() != null && (c.getHigh() == null || candle.getHigh() > c.getHigh())) c.setHigh(candle.getHigh());
                    if (candle.getLow() != null && (c.getLow() == null || candle.getLow() < c.getLow())) c.setLow(candle.getLow());
                    if (candle.getClose() != null) c.setClose(candle.getClose());
                    if (candle.getVolume() != null) c.setVolume(candle.getVolume());
                } else {
                    list.add(candle);
                }
                lastEventAt.set(System.currentTimeMillis());
            };

            // Abrir los canales con un pequeno stagger (no todos de golpe): abrir
            // 10 canales nuevos instantaneamente resulto no ser confiable -- al
            // menos uno no completaba el handshake CHANNEL_REQUEST/FEED_CONFIG
            // dentro del timeout. Cada apertura es independiente: si una falla,
            // se salta y se sigue con las demas en vez de tumbar todo el lote.
            for (int c = 0; c < channelCount; c++) {
                try {
                    DxLinkClient.DxLinkChannel ch = dxLinkClient.openNewChannel().get(10, TimeUnit.SECONDS);
                    openedChannels.add(ch);
                } catch (Exception e) {
                    log.warn("Candle channel {}/{} failed to open: {}", c + 1, channelCount, e.getMessage());
                }
                if (c < channelCount - 1) Thread.sleep(150);
            }
            if (openedChannels.isEmpty()) {
                log.error("Candles failed: no channel could be opened");
                return Map.of();
            }
            // addCandleListener registra en una lista global del cliente (no por
            // canal), asi que basta con una sola registracion para recibir eventos
            // de los N canales -- registrarlo en cada uno duplicaria el trabajo.
            openedChannels.get(0).addCandleListener(sharedListener);

            int openedCount = openedChannels.size();
            List<String> symList = new ArrayList<>(symbols);
            int perChannel = (int) Math.ceil(symList.size() / (double) openedCount);
            for (int c = 0; c < openedCount; c++) {
                int groupStart = c * perChannel;
                if (groupStart >= symList.size()) break;
                int groupEnd = Math.min(groupStart + perChannel, symList.size());
                List<String> group = symList.subList(groupStart, groupEnd);
                DxLinkClient.DxLinkChannel ch = openedChannels.get(c);
                for (int i = 0; i < group.size(); i += 10) {
                    int end = Math.min(i + 10, group.size());
                    List<Map<String, Object>> items = new java.util.ArrayList<>();
                    for (String s : group.subList(i, end)) {
                        Map<String, Object> item = new java.util.HashMap<>();
                        item.put("symbol", String.format("%s{=%s%s}", s, period, type));
                        item.put("type", "Candle");
                        items.add(item);
                    }
                    ch.subscribeCandlesHistory(items, fromTime.toEpochMilli());
                }
            }
            int timeoutSec = Math.min(10 + symbols.size() / 10, 30);
            long deadline = System.currentTimeMillis() + timeoutSec * 1000L;
            while (System.currentTimeMillis() < deadline) {
                Thread.sleep(200);
                long last = lastEventAt.get();
                if (last > 0 && System.currentTimeMillis() - last > CANDLE_QUIET_PERIOD_MS) break;
            }
            return resultado;
        } catch (Exception e) {
            log.error("Candles failed: {}", e.getMessage());
            return resultado.isEmpty() ? Map.of() : resultado;
        } finally {
            for (var ch : openedChannels) ch.close();
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
     * Obtiene métricas fundamentales vía REST (Market Metrics + Market Data OHLC).
     * DxLink Profile/Summary snapshot no escala a miles de símbolos (~1.7% de éxito
     * medido en vivo con 9034 símbolos) y todos sus campos ya vienen cubiertos por REST.
     */
    public Map<String, FundamentalData> getFundamentalsBatch(List<String> symbols) {
        List<String> normalizedSymbols = symbols.stream()
                .map(String::toUpperCase)
                .distinct()
                .toList();

        log.info("Batch fundamentals: {} simbolos via REST", normalizedSymbols.size());

        // Estimado de Market Cap con prevClose (el enriquecimiento REST más abajo
        // lo reemplaza por el market-cap oficial de TastyTrade si está disponible)
        normalizedSymbols.forEach(sym -> {
            FundamentalData fund = fundamentalsCache.get(sym);
            if (fund != null && fund.getPrevClose() != null && fund.getSharesOutstanding() != null) {
                fund.setMarketCap(fund.getSharesOutstanding() * fund.getPrevClose());
            }
        });

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

                Double metricShortRatio = safeConvertToDouble(metric.get("short-ratio"));
                if (metricShortRatio != null) fund.setShortRatio(metricShortRatio);

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

        // Enriquecimiento con Market Data OHLC en BATCH (REST): precio, volumen, beta y estado de halt
        try {
            List<Map<String, Object>> ohlcItems = getMarketDataBatch(normalizedSymbols);
            for (Map<String, Object> item : ohlcItems) {
                String sym = (String) item.get("symbol");
                if (sym == null) continue;
                FundamentalData fund = fundamentalsCache.get(sym.toUpperCase());
                if (fund != null) {
                    if (fund.getOpen() == null) fund.setOpen(safeConvertToDouble(item.get("open")));
                    if (fund.getHigh() == null) fund.setHigh(safeConvertToDouble(item.get("day-high-price")));
                    if (fund.getLow() == null) fund.setLow(safeConvertToDouble(item.get("day-low-price")));
                    if (fund.getPrevClose() == null) fund.setPrevClose(safeConvertToDouble(item.get("prev-close")));
                    if (fund.getDayVolume() == null) fund.setDayVolume(safeConvertToLong(item.get("volume")));
                    if (fund.getBeta() == null) fund.setBeta(safeConvertToDouble(item.get("beta")));
                    if (fund.getTradingStatus() == null && item.get("is-trading-halted") != null) {
                        fund.setTradingStatus(Boolean.TRUE.equals(item.get("is-trading-halted")) ? "HALTED" : "ACTIVE");
                    }
                    if (fund.getHaltStartTime() == null) fund.setHaltStartTime(safeConvertToLong(item.get("halt-start-time")));
                    if (fund.getHaltEndTime() == null) fund.setHaltEndTime(safeConvertToLong(item.get("halt-end-time")));
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
            log.warn("DxLink disconnected: {} — reconnecting", dxLinkClient.connectionDiagnostics());
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
            dxLinkClient.getDefaultChannel().setOnMarketData((symbol, data) -> {
                if (data.getLastPrice() != null && data.getLastPrice() > 0) {
                    lastPricesCache.put(symbol.toUpperCase(), data.getLastPrice());
                }
            });
            dxLinkClient.resubscribeAllSymbols();
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
