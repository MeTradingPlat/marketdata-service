package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import java.time.Instant;
import java.time.LocalDate;
import java.time.temporal.ChronoUnit;
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

import java.util.concurrent.RejectedExecutionException;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import java.util.concurrent.atomic.AtomicReference;
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
    private static final int CHUNK_SIZE = 100;

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
     * Obtiene candles históricos de múltiples símbolos usando UN canal DxLink.
     * Las FEED_SUBSCRIPTION son aditivas (spec dxLink): enviamos N mensajes de 200 símbolos
     * al mismo canal y se acumulan. CHANNEL_CANCEL cierra el canal al final para evitar
     * canales fantasma que causan INVALID_MESSAGE en batches posteriores.
     */
    public Map<String, List<Candle>> getCandlesBatch(List<String> symbols, EnumTimeframe timeframe, int bars) {
        log.info("Batch fetch: {} simbolos, timeframe={}, bars={}", symbols.size(), timeframe, bars);

        ensureConnected();

        Instant now = Instant.now();
        long fromTime = now.minus(timeframe.getDuration().multipliedBy(bars + 10)).toEpochMilli();
        String tf = timeframe.getLabel();

        DxLinkClient.DxLinkChannel channel;
        try {
            channel = dxLinkClient.openNewChannel().get(10, TimeUnit.SECONDS);
        } catch (Exception e) {
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
                } catch (RejectedExecutionException ignored) {
                }
            } else {
                ScheduledFuture<?> pending = settleTask.get();
                if (pending != null) pending.cancel(false);
            }
        };

        channel.addCandleListener(listener);

        // Enviar N × FEED_SUBSCRIPTION aditivas al mismo canal (200 símbolos c/u, ≤64KB).
        List<List<String>> chunks = partition(symbols, CHUNK_SIZE);
        log.info("Canal {}: suscribiendo {} simbolos en {} mensajes FEED_SUBSCRIPTION",
                channel.getId(), symbols.size(), chunks.size());
        for (List<String> chunk : chunks) {
            List<Map<String, Object>> items = chunk.stream()
                    .map(s -> Map.<String, Object>of(
                            "symbol", s + "{=" + tf + "}",
                            "type", "Candle",
                            "fromTime", fromTime))
                    .toList();
            channel.subscribeCandlesBatch(items);
        }

        // Timeout dinámico: scanners (≤100 syms) = 8s, batches grandes escalan.
        // Con mercado cerrado muchos batches no reciben nada; 8s evita bloquear
        // la cola 30s por cada scanner sin datos.
        int maxWaitSeconds = symbols.size() <= CHUNK_SIZE
                ? 8
                : Math.min(30 + symbols.size() / 200, 120);
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
                log.info("Batch timeout {}s en canal {}. Recibidos {}/{} — resultados parciales.",
                        maxWaitSeconds, channel.getId(), received, symbols.size());
            }
        } catch (Exception e) {
            log.error("Batch interrupted", e);
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

    public Map<String, FundamentalData> getFundamentalsBatch(List<String> symbols) {
        log.info("Batch fundamentals: {} simbolos", symbols.size());
        ensureConnected();

        DxLinkClient.DxLinkChannel channel;
        try {
            channel = dxLinkClient.openNewChannel().get(10, TimeUnit.SECONDS);
        } catch (Exception e) {
            log.error("No se pudo abrir canal DxLink para fundamentals: {}", e.getMessage());
            return Map.of();
        }

        ConcurrentHashMap<String, FundamentalData> fundamentalsMap = new ConcurrentHashMap<>();
        CompletableFuture<Void> allCompleted = new CompletableFuture<>();
        ScheduledExecutorService scheduler = Executors.newSingleThreadScheduledExecutor();

        BiConsumer<String, FundamentalData> listener = (sym, data) -> {
            fundamentalsMap.compute(sym, (k, v) -> {
                if (v == null) return data;
                if (data.getDayVolume() != null) v.setDayVolume(data.getDayVolume());
                if (data.getMarketCap() != null) v.setMarketCap(data.getMarketCap());
                if (data.getSharesOutstanding() != null) v.setSharesOutstanding(data.getSharesOutstanding());
                if (data.getFloatShares() != null) v.setFloatShares(data.getFloatShares());
                if (data.getShortInterest() != null) v.setShortInterest(data.getShortInterest());
                if (data.getPreMarketVolume() != null) v.setPreMarketVolume(data.getPreMarketVolume());
                if (data.getPostMarketVolume() != null) v.setPostMarketVolume(data.getPostMarketVolume());
                return v;
            });

            // Dejamos de esperar marketCap y dayVolume obligatoriamente de DxLink, ya que pueden tardar o no venir.
            // El batch terminara por timeout o porque llegamos al numero de simbolos.
            if (fundamentalsMap.size() >= symbols.size()) {
                // Si ya tenemos todos los simbolos en el mapa, podemos considerar el batch "suficiente"
                // aunque no tengan todos los campos, para no bloquear el hilo innecesariamente.
                allCompleted.complete(null);
            }
        };

        channel.addFundamentalListener(listener);
        channel.subscribeFundamentalsBatch(symbols);
        channel.subscribeExtendedVolumeBatch(symbols);

        try {
            // Timeout mas corto para fundamentals ya que son snapshots
            allCompleted.get(Math.min(10 + symbols.size() / 100, 30), TimeUnit.SECONDS);
        } catch (TimeoutException e) {
            log.warn("Batch fundamentals partial results: {}/{}", fundamentalsMap.size(), symbols.size());
        } catch (Exception e) {
            log.error("Batch fundamentals error", e);
        }

        scheduler.shutdownNow();
        channel.close();

        // Mezclar con Market Metrics de la API REST para campos faltantes (shortRatio, earnings)
        try {
            List<Map<String, Object>> metrics = getMarketMetricsBatch(symbols);
            for (Map<String, Object> metric : metrics) {
                String sym = (String) metric.get("symbol");
                if (sym == null) continue;

                FundamentalData fund = fundamentalsMap.computeIfAbsent(sym, k -> FundamentalData.builder().symbol(k).build());

                // Populating Short Data
                Object shortRatioValue = metric.get("short-ratio");
                if (shortRatioValue instanceof Number) {
                    fund.setShortRatio(((Number) shortRatioValue).doubleValue());
                }
                Object shortInterestValue = metric.get("short-interest");
                if (shortInterestValue instanceof Number) {
                    fund.setShortInterest(((Number) shortInterestValue).doubleValue());
                }

                // Populating Financial Metrics
                Object marketCapValue = metric.get("market-cap");
                if (marketCapValue instanceof Number) {
                    fund.setMarketCap(((Number) marketCapValue).doubleValue());
                }
                Object sharesOutstandingValue = metric.get("shares-outstanding");
                if (sharesOutstandingValue instanceof Number) {
                    fund.setSharesOutstanding(((Number) sharesOutstandingValue).longValue());
                }
                Object freeFloatValue = metric.get("free-float");
                if (freeFloatValue instanceof Number) {
                    fund.setFloatShares(((Number) freeFloatValue).longValue());
                }

                // Populating daysUntilEarnings
                Object earningsDateValue = metric.get("earnings-report-date");
                if (earningsDateValue instanceof String) {
                    try {
                        LocalDate earningsDate = LocalDate.parse((String) earningsDateValue);
                        long days = ChronoUnit.DAYS.between(LocalDate.now(), earningsDate);
                        fund.setDaysUntilEarnings((int) Math.max(0, days));
                    } catch (Exception e) {
                        log.debug("Failed to parse earnings date for {}: {}", sym, earningsDateValue);
                    }
                }
            }
        } catch (Exception e) {
            log.error("Failed to merge market metrics in fundamentals batch: {}", e.getMessage());
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
