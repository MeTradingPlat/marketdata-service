package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import java.time.Instant;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
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
     * Obtiene candles históricos directamente de DxLink.
     * Retorna todas las candles que DxLink envía (~700 max), ordenadas por timestamp ascendente.
     */
    public List<Candle> getCandles(String symbol, EnumTimeframe timeframe) {
        log.info("Fetching candles for {} {}", symbol, timeframe);
        return fetchCandlesFromDxLink(symbol, timeframe);
    }

    private List<Candle> fetchCandlesFromDxLink(String symbol, EnumTimeframe timeframe) {
        ensureConnected();

        // Usar Set para evitar duplicados
        Set<CandleKey> seenCandles = ConcurrentHashMap.newKeySet();
        List<Candle> receivedCandles = new ArrayList<>();

        // Configurar callback temporal para recibir candles
        dxLinkClient.setOnCandle((sym, candle) -> {
            log.debug("Candle callback: sym={}, symbol={}, match={}", sym, symbol, sym.equals(symbol));
            if (sym.equals(symbol) || sym.startsWith(symbol)) {
                candle.setSymbol(symbol);
                candle.setTimeframe(timeframe);

                CandleKey key = new CandleKey(symbol, timeframe, candle.getTimestamp());
                if (seenCandles.add(key)) {
                    synchronized (receivedCandles) {
                        receivedCandles.add(candle);
                    }
                    log.info("Added candle #{} to list: {} at {} O={} H={} L={} C={} V={}",
                        receivedCandles.size(), symbol, candle.getTimestamp(), candle.getOpen(),
                        candle.getHigh(), candle.getLow(), candle.getClose(), candle.getVolume());
                } else {
                    log.debug("Skipping duplicate candle: {} at {}", symbol, candle.getTimestamp());
                }
            }
        });

        // Resetear estado del snapshot y suscribir a candles históricos
        dxLinkClient.resetCandleSnapshot();
        String tf = timeframe.getLabel();
        // Calcular fromTime: ~800 barras hacia atras para cubrir las ~700 que DxLink envia
        long fromTime = Instant.now().minus(timeframe.getDuration().multipliedBy(800)).toEpochMilli();
        log.info("Subscribing to candles: symbol={}, timeframe={}, fromTime={}", symbol, tf, Instant.ofEpochMilli(fromTime));
        dxLinkClient.subscribeCandles(symbol, tf, fromTime);

        // Esperar hasta que el snapshot esté completo O timeout
        int waitSeconds = 0;
        int maxWaitSeconds = 30;
        int noNewCandlesCount = 0;
        int lastCount = 0;

        while (waitSeconds < maxWaitSeconds) {
            try {
                Thread.sleep(1000);
                waitSeconds++;

                int currentCount = dxLinkClient.getCandleSnapshotCount();
                boolean snapshotComplete = dxLinkClient.isCandleSnapshotComplete();

                log.info("Waiting for candles... {}s elapsed, {} candles received, snapshotComplete={}",
                    waitSeconds, currentCount, snapshotComplete);

                if (snapshotComplete) {
                    // Esperar 2 segundos para que batches en vuelo terminen de llegar
                    // (DxLink envia en multiples frames/threads simultaneos)
                    log.info("Snapshot signal received at {} candles, waiting 2s for remaining batches...", currentCount);
                    Thread.sleep(2000);
                    log.info("Snapshot complete after {}s, {} unique candles (raw events: {})",
                        waitSeconds, receivedCandles.size(), dxLinkClient.getCandleSnapshotCount());
                    break;
                }

                if (currentCount > 0 && currentCount == lastCount) {
                    noNewCandlesCount++;
                    if (noNewCandlesCount >= 3) {
                        log.info("No new candles in 3 seconds, assuming complete");
                        break;
                    }
                } else {
                    noNewCandlesCount = 0;
                }
                lastCount = currentCount;

            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                break;
            }
        }

        // Desuscribir de candles
        dxLinkClient.unsubscribeCandles(symbol, tf);

        // Restaurar callback por defecto (solo logging)
        dxLinkClient.setOnCandle((sym, candle) -> {
            log.debug("Candle received for {}: {} O={} H={} L={} C={}",
                    sym, candle.getTimestamp(), candle.getOpen(),
                    candle.getHigh(), candle.getLow(), candle.getClose());
        });

        log.info("Finished waiting after {}s. Received {} unique candles from DxLink for {}",
            waitSeconds, receivedCandles.size(), symbol);

        // Ordenar por timestamp ascendente
        List<Candle> sorted;
        synchronized (receivedCandles) {
            sorted = receivedCandles.stream()
                .sorted(Comparator.comparing(Candle::getTimestamp))
                .collect(Collectors.toList());
        }

        log.info("Returning {} candles for {} {}", sorted.size(), symbol, timeframe);
        return sorted;
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
