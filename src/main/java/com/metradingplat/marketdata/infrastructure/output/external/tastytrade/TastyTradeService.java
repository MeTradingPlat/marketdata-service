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
    private final java.util.concurrent.ScheduledExecutorService scheduler = java.util.concurrent.Executors.newSingleThreadScheduledExecutor();

    // Trackers para la heuristica de Halt Status (Punto 5)
    private final ConcurrentHashMap<String, Long> lastMarketDataUpdates = new ConcurrentHashMap<>();
    private final ConcurrentHashMap<String, OptionContract> greeksCache = new ConcurrentHashMap<>();

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
            
            if (token != null && url != null) {
                log.debug("Got token and URL. Connecting to DxLink at: {}", url);
                dxLinkClient.connect(url, token);
            } else {
                log.warn("TastyTrade token or URL is null. Skipping DxLink connection (likely in test environment).");
            }
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

        dxLinkClient.setOnGreeks((symbol, greeks) -> {
            log.debug("Greeks received for {}: Delta={}, IV={}", symbol, greeks.getDelta(), greeks.getImpliedVolatility());
            greeksCache.put(symbol, greeks);
            // Aquí se podría publicar a Kafka si fuera necesario
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
            
            channel.addCandleListener((symbol, candle, isSnapshotComplete) -> {
                candle.setTimeframe(timeframe);
                resultado.computeIfAbsent(symbol, k -> new java.util.ArrayList<>()).add(candle);
            });

            List<Map<String, Object>> subscriptionItems = symbols.stream()
                .map(s -> {
                    // Sintaxis dxLink: Symbol{=periodType} ej: AAPL{=5m}
                    String dxSymbol = String.format("%s{=%s%s}", s, period, type);
                    Map<String, Object> item = new java.util.HashMap<>();
                    item.put("symbol", dxSymbol);
                    item.put("type", "Candle");
                    return item;
                })
                .toList();

            channel.subscribeCandlesHistory(subscriptionItems, fromTime.toEpochMilli());

            // Esperamos a que se llene el snapshot
            scheduler.schedule(() -> {
                channel.close();
                
                // RECORTE FINAL (Simulando el parámetro 'limit')
                resultado.forEach((symbol, candles) -> {
                    // Ordenar por tiempo (por si acaso dxLink los manda desordenados, aunque no suele pasar)
                    candles.sort(java.util.Comparator.comparing(Candle::getTimestamp));
                    
                    // Si tenemos más de las pedidas, nos quedamos con las últimas 'bars'
                    if (candles.size() > bars) {
                        List<Candle> truncated = new java.util.ArrayList<>(candles.subList(candles.size() - bars, candles.size()));
                        resultado.put(symbol, truncated);
                    }
                });
                
                future.complete(resultado);
            }, 3 + (symbols.size() / 10), TimeUnit.SECONDS);

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

        ConcurrentHashMap<String, FundamentalData> fundamentalsMap = new ConcurrentHashMap<>();
        ConcurrentHashMap<String, Double> lastPrices = new ConcurrentHashMap<>();
        CompletableFuture<Void> snapshotReceived = new CompletableFuture<>();
        Set<String> symbolsWithProfile = ConcurrentHashMap.newKeySet();

        try (DxLinkClient.DxLinkChannel channel = dxLinkClient.openNewChannel().get(10, TimeUnit.SECONDS)) {
            
            // Listener para capturar todo: Profile, Summary y Quotes (para el Market Cap)
            channel.addFundamentalListener((sym, data) -> {
                String upperSym = sym.toUpperCase();
                fundamentalsMap.merge(upperSym, data, (v1, v2) -> {
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

                // Si recibimos acciones, marcamos como perfil recibido
                if (data.getSharesOutstanding() != null) {
                    symbolsWithProfile.add(upperSym);
                    if (symbolsWithProfile.size() >= normalizedSymbols.size()) {
                        snapshotReceived.complete(null);
                    }
                }
            });

            // También escuchamos el precio en vivo para calcular el Market Cap (Punto 2 del notebook)
            channel.addMarketDataListener((sym, data) -> {
                if (data.getLastPrice() != null) {
                    lastPrices.put(sym.toUpperCase(), data.getLastPrice());
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
            fundamentalsMap.forEach((sym, fund) -> {
                Double lastPrice = lastPrices.get(sym);
                Double basePrice = (lastPrice != null) ? lastPrice : fund.getPrevClose();
                
                if (basePrice != null && fund.getSharesOutstanding() != null) {
                    fund.setMarketCap(fund.getSharesOutstanding() * basePrice);
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
                
                FundamentalData fund = fundamentalsMap.computeIfAbsent(finalSym, k -> FundamentalData.builder().symbol(k).build());

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
                FundamentalData fund = fundamentalsMap.get(sym.toUpperCase());
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
                FundamentalData fund = fundamentalsMap.get(sym.toUpperCase());
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
        
        // Conexión al Account Streamer
        String accessToken = tastyTradeClient.getAccessToken();
        String streamerUrl = tastyTradeClient.getAccountStreamerUrl();
        accountStreamerClient.connect(streamerUrl, accessToken);
    }

    private Double safeConvertToDouble(Object val) {
        if (val == null) return null;
        if (val instanceof Number) return ((Number) val).doubleValue();
        try {
            return Double.parseDouble(val.toString());
        } catch (Exception e) {
            return null;
        }
    }

    private Long safeConvertToLong(Object val) {
        if (val == null) return null;
        if (val instanceof Number) return ((Number) val).longValue();
        try {
            return Long.parseLong(val.toString());
        } catch (Exception e) {
            return null;
        }
    }

    private Integer safeConvertToInt(Object val) {
        if (val == null) return null;
        if (val instanceof Number) return ((Number) val).intValue();
        try {
            return Integer.parseInt(val.toString());
        } catch (Exception e) {
            return null;
        }
    }
}
