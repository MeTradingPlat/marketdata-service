package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;
import jakarta.annotation.PostConstruct;
import jakarta.annotation.PreDestroy;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;
import java.util.function.Consumer;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * Pool elastico de conexiones/canales DxLink persistentes para velas.
 * Reemplaza el canal unico + las rafagas efimeras que existian antes: cualquier
 * simbolo pedido (uno o miles) se coloca en un canal con espacio y se queda
 * suscrito, actualizandose en vivo hasta que CandleIdleEvictor lo de de baja
 * por inactividad real.
 */
@Slf4j
@Component
@RequiredArgsConstructor
public class CandleSubscriptionPool {

    private final CandleCacheStore cacheStore;
    private final CandleLiveListenerRegistry liveListeners;
    private final CandleChannelAllocator allocator;
    private final CandleIdleEvictor idleEvictor;
    private final CandleHistorySubscriber historySubscriber;
    private final List<PooledCandleConnection> connections = new CopyOnWriteArrayList<>();
    private final ScheduledExecutorService scheduler = Executors.newSingleThreadScheduledExecutor();
    private volatile Consumer<Candle> onEveryCandle;
    // No puede ser un inicializador de campo: Lombok asigna los campos
    // inyectados por constructor DESPUES de que corren los inicializadores de
    // campo, asi que un lambda inline aqui referenciando cacheStore/liveListeners
    // fallaria "might not have been initialized" en el analisis de asignacion
    // definitiva del compilador. Se arma en init(), cuando ya estan asignados.
    private DxLinkClient.CandleCallback mergeListener;

    @PostConstruct
    private void init() {
        mergeListener = (symbol, candle, isSnapshotComplete) -> {
            String cleanSymbol = symbol.replaceAll("\\{=.*\\}", "");
            Matcher tfMatcher = Pattern.compile("\\{=(.*?)\\}").matcher(symbol);
            EnumTimeframe tf = parseTimeframeFromLabel(tfMatcher.find() ? tfMatcher.group(1) : null);
            if (tf == null) return;
            candle.setSymbol(cleanSymbol);
            candle.setTimeframe(tf);
            String key = CandleCacheStore.key(cleanSymbol, tf);
            // Un tick llegando NO cuenta como "alguien lo esta pidiendo": el
            // reloj de inactividad (ver touchIfSubscribed) solo se resetea con
            // peticiones reales de un consumidor, para que un simbolo liquido
            // que nadie consulta ya se pueda desuscribir igual. Los datos
            // siguen quedando cacheados para cuando alguien pregunte.
            Candle merged = cacheStore.merge(key, candle);
            liveListeners.notify(key, merged);
            if (onEveryCandle != null) onEveryCandle.accept(merged);
        };
        scheduler.scheduleAtFixedRate(() -> idleEvictor.evict(connections), 5, 5, TimeUnit.MINUTES);
    }

    /**
     * Callback global disparado por cada vela mergeada de cualquier simbolo,
     * suscrito o no a un listener por-simbolo -- usado por TastyTradeService
     * para recalcular marketCap con el precio en vivo (antes vivia inline en
     * ensureCandleChannel()).
     */
    public void setOnEveryCandle(Consumer<Candle> callback) {
        this.onEveryCandle = callback;
    }

    public int availableConnectionPermits() {
        return allocator.availablePermits();
    }

    public Map<String, List<Candle>> getCandles(List<String> symbols, EnumTimeframe timeframe, int bars,
            Instant fromTime, String period, String type) {
        ensureSubscribedBatch(symbols, timeframe, fromTime, period, type);
        Map<String, List<Candle>> result = new HashMap<>();
        for (String sym : symbols) {
            List<Candle> candles = cacheStore.get(CandleCacheStore.key(sym, timeframe), bars);
            if (!candles.isEmpty()) result.put(sym.toUpperCase(), candles);
        }
        return result;
    }

    public void addLiveListener(String symbol, EnumTimeframe timeframe, Consumer<Candle> listener) {
        liveListeners.add(CandleCacheStore.key(symbol, timeframe), listener);
    }

    public void removeLiveListener(String symbol, EnumTimeframe timeframe, Consumer<Candle> listener) {
        liveListeners.remove(CandleCacheStore.key(symbol, timeframe), listener);
    }

    private void ensureSubscribedBatch(List<String> symbols, EnumTimeframe timeframe, Instant fromTime,
            String period, String type) {
        List<String> newSymbols = new ArrayList<>();
        for (String sym : symbols) {
            if (!touchIfSubscribed(CandleCacheStore.key(sym, timeframe))) newSymbols.add(sym);
        }
        if (newSymbols.isEmpty()) return;
        log.info("Candle pool onboarding {} new symbols, timeframe={}", newSymbols.size(), timeframe);
        allocator.ensureCapacityFor(connections, newSymbols.size(), mergeListener);
        List<String> onboardedKeys = new ArrayList<>();
        for (String sym : newSymbols) {
            PooledCandleChannel ch = allocator.allocate(connections, mergeListener);
            if (ch == null) {
                log.warn("Could not place {} in the candle pool -- skipping, will retry on a future request", sym);
                continue;
            }
            String key = CandleCacheStore.key(sym, timeframe);
            ch.touch(key);
            historySubscriber.subscribe(List.of(ch.channel), List.of(sym), period, type, fromTime);
            onboardedKeys.add(key);
        }
        awaitHistory(onboardedKeys);
    }

    // Unica fuente del reloj de inactividad: se llama solo desde una peticion
    // real de un consumidor (ensureSubscribedBatch), nunca desde el merge de
    // un tick en vivo.
    private boolean touchIfSubscribed(String key) {
        for (PooledCandleConnection conn : connections) {
            for (PooledCandleChannel ch : conn.channels) {
                if (ch.contains(key)) { ch.touch(key); return true; }
            }
        }
        return false;
    }

    private void awaitHistory(List<String> keys) {
        if (keys.isEmpty()) return;
        int timeoutSec = Math.min(10 + keys.size() / 10, 30);
        long deadline = System.currentTimeMillis() + timeoutSec * 1000L;
        while (System.currentTimeMillis() < deadline) {
            if (keys.stream().allMatch(cacheStore::hasData)) return;
            try { Thread.sleep(200); } catch (InterruptedException e) { Thread.currentThread().interrupt(); return; }
        }
    }

    // dxFeed omite el "1" de periodo en el symbol que devuelve para candles de
    // periodo 1 (ej. suscribimos "AAPL{=1m}" pero el FEED_DATA que llega trae
    // "AAPL{=m}") -- ver historial de esta sesion en TastyTradeService (donde
    // vivia esta misma logica antes de moverse aca con el resto del pool).
    private static EnumTimeframe parseTimeframeFromLabel(String label) {
        if (label == null) return null;
        for (EnumTimeframe tf : EnumTimeframe.values()) {
            String canonical = tf.getLabel();
            String withoutImplicitPeriod = canonical.startsWith("1") ? canonical.substring(1) : canonical;
            if (label.equals(canonical) || label.equals(withoutImplicitPeriod)) return tf;
        }
        return null;
    }

    @PreDestroy
    public void shutdown() {
        scheduler.shutdown();
        connections.forEach(c -> c.client.disconnect());
    }
}
