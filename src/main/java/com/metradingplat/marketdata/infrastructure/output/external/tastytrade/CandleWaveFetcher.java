package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicLong;

/**
 * Corre el protocolo de historico de velas (abrir canales, suscribir, esperar
 * el quiet-period, cerrar) contra un DxLinkClient recibido por parametro en
 * vez de un campo fijo, para poder reutilizarlo tanto con la conexion
 * principal como con conexiones efimeras abiertas por CandleBurstOrchestrator.
 */
@Slf4j
@Component
@RequiredArgsConstructor
public class CandleWaveFetcher {

    // Ver TastyTradeService (historial de esta sesion) para el detalle empirico
    // de estos numeros: ~100 suscripciones aceptadas por canal, ~8 canales
    // nuevos por conexion -> 800 simbolos por oleada como techo duro.
    private static final int CHANNEL_SPLIT_THRESHOLD = 150;
    private static final int SYMBOLS_PER_CHANNEL = 100;
    private static final int MAX_CHANNELS = 8;
    private static final long QUIET_PERIOD_MS = 1000;
    public static final int WAVE_MAX_SYMBOLS = MAX_CHANNELS * SYMBOLS_PER_CHANNEL;

    private final CandleChannelOpener channelOpener;
    private final CandleHistorySubscriber historySubscriber;

    public Map<String, List<Candle>> fetchAllWaves(DxLinkClient client, List<String> symbols,
            EnumTimeframe timeframe, Instant fromTime, String period, String type) {
        Map<String, List<Candle>> resultado = new ConcurrentHashMap<>();
        List<String> remaining = new ArrayList<>(symbols);
        int wave = 0;
        while (!remaining.isEmpty()) {
            int waveSize = Math.min(remaining.size(), WAVE_MAX_SYMBOLS);
            List<String> waveSymbols = new ArrayList<>(remaining.subList(0, waveSize));
            remaining.subList(0, waveSize).clear();
            wave++;
            log.info("Candle wave {}: {} symbols ({} remaining)", wave, waveSymbols.size(), remaining.size());
            fetchWave(client, waveSymbols, timeframe, period, type, fromTime, resultado);
        }
        return resultado;
    }

    private void fetchWave(DxLinkClient client, List<String> symbols, EnumTimeframe timeframe, String period,
            String type, Instant fromTime, Map<String, List<Candle>> resultado) {
        int channelCount = symbols.size() > CHANNEL_SPLIT_THRESHOLD
                ? Math.min(MAX_CHANNELS, (symbols.size() + SYMBOLS_PER_CHANNEL - 1) / SYMBOLS_PER_CHANNEL)
                : 1;
        AtomicLong lastEventAt = new AtomicLong(0);
        DxLinkClient.CandleCallback sharedListener = CandleMergeListener.forResult(timeframe, resultado, lastEventAt);
        List<DxLinkClient.DxLinkChannel> openedChannels = new ArrayList<>();
        try {
            openedChannels.addAll(channelOpener.open(client, channelCount));
            if (openedChannels.isEmpty()) {
                log.error("Candle wave failed: no channel could be opened");
                return;
            }
            // addCandleListener registra en una lista global del cliente (no por
            // canal), asi que basta con una sola registracion para recibir eventos
            // de los N canales abiertos en esta oleada.
            openedChannels.get(0).addCandleListener(sharedListener);
            historySubscriber.subscribe(openedChannels, symbols, period, type, fromTime);
            awaitQuiet(symbols.size(), lastEventAt);
        } catch (Exception e) {
            log.error("Candle wave failed: {}", e.getMessage());
        } finally {
            for (var ch : openedChannels) ch.close();
            if (!openedChannels.isEmpty()) openedChannels.get(0).removeCandleListener(sharedListener);
        }
    }

    private void awaitQuiet(int symbolCount, AtomicLong lastEventAt) throws InterruptedException {
        int timeoutSec = Math.min(10 + symbolCount / 10, 30);
        long deadline = System.currentTimeMillis() + timeoutSec * 1000L;
        while (System.currentTimeMillis() < deadline) {
            Thread.sleep(200);
            long last = lastEventAt.get();
            if (last > 0 && System.currentTimeMillis() - last > QUIET_PERIOD_MS) break;
        }
    }
}
