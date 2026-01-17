package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import java.time.Instant;
import java.time.OffsetDateTime;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;

import org.springframework.stereotype.Service;

import com.metradingplat.marketdata.application.output.GestionarChangeNotificationsProducerIntPort;
import com.metradingplat.marketdata.application.output.GestionarHistoricalDataGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.domain.models.OrderRequest;

import jakarta.annotation.PostConstruct;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

/**
 * Servicio interno de TastyTrade.
 * Orquesta TastyTradeClient (REST) y DxLinkClient (WebSocket).
 * No implementa la interfaz directamente - eso lo hace
 * GestionarComunicacionExternalGatewayImplAdapter.
 */
@Service
@RequiredArgsConstructor
@Slf4j
public class TastyTradeService {

    private final TastyTradeClient tastyTradeClient;
    private final DxLinkClient dxLinkClient;
    private final GestionarChangeNotificationsProducerIntPort kafkaProducer;
    private final GestionarHistoricalDataGatewayIntPort historicalDataGateway;

    @PostConstruct
    public void init() {
        log.info("Initializing TastyTrade service");

        // Configurar callback para datos de mercado → Kafka
        dxLinkClient.setOnMarketData((symbol, data) -> {
            log.debug("Market data received for {}: bid={}, ask={}, last={}",
                    symbol, data.getBid(), data.getAsk(), data.getLastPrice());
            kafkaProducer.publishMarketData(data);
        });

        // Configurar callback para candles → guardar en BD
        dxLinkClient.setOnCandle((symbol, candle) -> {
            log.debug("Candle received for {}: {} O={} H={} L={} C={}",
                    symbol, candle.getTimestamp(), candle.getOpen(),
                    candle.getHigh(), candle.getLow(), candle.getClose());
            historicalDataGateway.saveCandles(List.of(candle));
        });

        // Configurar token refresher para auto-reconexión
        // Esto permite que DxLinkClient obtenga un token fresco cuando reconecta
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
            // El DxLinkClient ahora maneja reconexión automáticamente
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

    public List<Candle> getCandles(String symbol, EnumTimeframe timeframe, OffsetDateTime from, OffsetDateTime to) {
        log.info("Fetching candles for {} {} from {} to {}", symbol, timeframe, from, to);

        // Primero buscar en BD
        long count = historicalDataGateway.countData(symbol, timeframe, from, to);
        long expected = calculateExpectedCandles(timeframe, from, to);

        if (count >= expected * 0.95) {
            log.info("Returning {} candles from database", count);
            return historicalDataGateway.getHistoricalData(symbol, timeframe, from, to);
        }

        // Si no hay suficientes, obtener de DxLink
        log.info("Insufficient data in DB ({}/{}), fetching from DxLink", count, expected);
        return fetchCandlesFromDxLink(symbol, timeframe, from, to);
    }

    private List<Candle> fetchCandlesFromDxLink(String symbol, EnumTimeframe timeframe,
            OffsetDateTime from, OffsetDateTime to) {
        ensureConnected();

        // Usar Set con comparador por timestamp para evitar duplicados
        Set<CandleKey> seenCandles = ConcurrentHashMap.newKeySet();
        List<Candle> receivedCandles = new ArrayList<>();

        // Configurar callback temporal para recibir candles
        dxLinkClient.setOnCandle((sym, candle) -> {
            log.debug("Candle callback: sym={}, symbol={}, match={}", sym, symbol, sym.equals(symbol));
            if (sym.equals(symbol) || sym.startsWith(symbol)) {
                candle.setSymbol(symbol); // Normalizar el símbolo
                candle.setTimeframe(timeframe);

                // Evitar duplicados por timestamp
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
        long fromTime = from.toInstant().toEpochMilli();
        log.info("Subscribing to candles: symbol={}, timeframe={}, fromTime={} ({})",
            symbol, tf, fromTime, from);
        dxLinkClient.subscribeCandles(symbol, tf, fromTime);

        // Esperar hasta que el snapshot esté completo O timeout (máximo 30 segundos)
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

                // Si el snapshot está completo, terminar
                if (snapshotComplete) {
                    log.info("Snapshot complete signal received after {}s", waitSeconds);
                    break;
                }

                // Si no llegan más candles en 3 segundos consecutivos, considerar terminado
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

        // Restaurar callback original
        dxLinkClient.setOnCandle((sym, candle) -> {
            historicalDataGateway.saveCandles(List.of(candle));
        });

        log.info("Finished waiting after {}s. Received {} unique candles from DxLink for {}",
            waitSeconds, receivedCandles.size(), symbol);

        // Guardar candles únicos en BD
        if (!receivedCandles.isEmpty()) {
            // Ordenar por timestamp antes de guardar
            receivedCandles.sort(Comparator.comparing(Candle::getTimestamp));
            historicalDataGateway.saveCandles(receivedCandles);
        }

        // Devolver lo que hay en BD (incluye los que acabamos de guardar)
        List<Candle> dbCandles = historicalDataGateway.getHistoricalData(symbol, timeframe, from, to);
        log.info("Returning {} candles from database for {}", dbCandles.size(), symbol);
        return dbCandles;
    }

    /**
     * Clave única para identificar un candle y evitar duplicados.
     */
    private record CandleKey(String symbol, EnumTimeframe timeframe, Instant timestamp) {}

    private void ensureConnected() {
        if (!dxLinkClient.isConnected()) {
            log.info("Reconnecting to DxLink");
            String token = tastyTradeClient.getApiQuoteToken();
            String url = tastyTradeClient.getDxlinkUrl();
            dxLinkClient.connect(url, token);
        }
    }

    private long calculateExpectedCandles(EnumTimeframe timeframe, OffsetDateTime from, OffsetDateTime to) {
        long minutes = java.time.Duration.between(from, to).toMinutes();
        return switch (timeframe) {
            case M1 -> minutes;
            case M5 -> minutes / 5;
            case M15 -> minutes / 15;
            case M30 -> minutes / 30;
            case H1 -> minutes / 60;
            case D1 -> java.time.Duration.between(from, to).toDays();
            case W1 -> java.time.Duration.between(from, to).toDays() / 7;
            case MO1 -> java.time.Duration.between(from, to).toDays() / 30;
        };
    }
}
