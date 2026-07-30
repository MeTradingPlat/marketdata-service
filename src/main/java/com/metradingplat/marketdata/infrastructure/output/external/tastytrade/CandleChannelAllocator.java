package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.List;
import java.util.Optional;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Semaphore;

/**
 * Decide donde colocar un simbolo+timeframe nuevo en el pool de velas:
 * primero un canal con espacio, luego un canal nuevo en una conexion con
 * espacio, y solo si ninguna tiene espacio, una conexion nueva -- hasta el
 * techo global configurado (mismo semaforo que antes limitaba las rafagas
 * efimeras, ahora limita cuantas conexiones persistentes puede tener el pool).
 * ensureCapacityFor abre de antemano, en paralelo (hilos virtuales), tantas
 * conexiones nuevas como haga falta para un lote grande -- lo lento es el
 * connect() de red, no la contabilidad de que simbolo va en que canal.
 */
@Slf4j
@Component
@RequiredArgsConstructor
class CandleChannelAllocator {

    // getApiQuoteToken() golpea /api-quote-tokens (REST real de TastyTrade) en
    // cada apertura de conexion, y es synchronized -- abrir varias conexiones
    // nuevas de golpe (onboarding grande) las sirve en fila pero sin ninguna
    // pausa, y eso ya disparo un rechazo de autenticacion en vivo dos veces
    // esta sesion. Mismo remedio que CandleChannelOpener ya usa para abrir
    // canales: un stagger chico entre lanzamientos, mas un reintento con
    // backoff antes de darse por vencido (el rechazo es transitorio, no
    // permanente -- reintentar casi siempre funciona).
    private static final long CONNECTION_STAGGER_MS = 200;
    private static final long CONNECTION_RETRY_BACKOFF_MS = 500;

    private final TastyTradeClient tastyTradeClient;
    private final TastyTradeConfig config;
    private final CandleChannelOpener channelOpener;
    private final ExecutorService onboardExecutor = Executors.newVirtualThreadPerTaskExecutor();
    private final Semaphore connectionBudget = new Semaphore(0);
    private volatile boolean budgetInitialized;

    int availablePermits() {
        ensureBudgetInitialized();
        return connectionBudget.availablePermits();
    }

    void releaseConnection() {
        connectionBudget.release();
    }

    private void ensureBudgetInitialized() {
        if (budgetInitialized) return;
        synchronized (this) {
            if (budgetInitialized) return;
            connectionBudget.release(config.getCandlePool().getMaxConcurrentConnections());
            budgetInitialized = true;
        }
    }

    void ensureCapacityFor(List<PooledCandleConnection> connections, int neededSlots,
            DxLinkClient.CandleCallback mergeListener) {
        ensureBudgetInitialized();
        int spare = spareCapacity(connections);
        if (spare >= neededSlots) return;
        int perConnection = PooledCandleConnection.MAX_CHANNELS * PooledCandleChannel.CAPACITY;
        int newConnectionsNeeded = Math.max(1, (int) Math.ceil((neededSlots - spare) / (double) perConnection));
        List<CompletableFuture<Void>> futures = new ArrayList<>();
        for (int i = 0; i < newConnectionsNeeded; i++) {
            if (!connectionBudget.tryAcquire()) {
                log.warn("Candle pool at its connection ceiling while onboarding a large batch -- proceeding with reduced capacity");
                break;
            }
            futures.add(CompletableFuture.runAsync(() -> {
                PooledCandleConnection conn = openConnectionWithRetry();
                if (conn == null) { connectionBudget.release(); return; }
                if (openChannel(conn, mergeListener).isEmpty()) { conn.client.disconnect(); connectionBudget.release(); return; }
                connections.add(conn);
            }, onboardExecutor));
            if (i < newConnectionsNeeded - 1) {
                try { Thread.sleep(CONNECTION_STAGGER_MS); } catch (InterruptedException e) { Thread.currentThread().interrupt(); break; }
            }
        }
        futures.forEach(CompletableFuture::join);
    }

    synchronized PooledCandleChannel allocate(List<PooledCandleConnection> connections,
            DxLinkClient.CandleCallback mergeListener) {
        ensureBudgetInitialized();
        for (PooledCandleConnection conn : connections) {
            for (PooledCandleChannel ch : conn.channels) {
                if (ch.hasRoom()) return ch;
            }
        }
        for (PooledCandleConnection conn : connections) {
            if (conn.hasRoomForNewChannel()) {
                Optional<PooledCandleChannel> opened = openChannel(conn, mergeListener);
                if (opened.isPresent()) return opened.get();
            }
        }
        if (!connectionBudget.tryAcquire()) {
            log.warn("Candle pool at its connection ceiling -- cannot place new symbol until something goes idle");
            return null;
        }
        PooledCandleConnection newConn = openConnectionWithRetry();
        if (newConn == null) {
            connectionBudget.release();
            return null;
        }
        connections.add(newConn);
        Optional<PooledCandleChannel> opened = openChannel(newConn, mergeListener);
        if (opened.isEmpty()) {
            connections.remove(newConn);
            connectionBudget.release();
            return null;
        }
        return opened.get();
    }

    private int spareCapacity(List<PooledCandleConnection> connections) {
        int spare = 0;
        for (PooledCandleConnection conn : connections) {
            for (PooledCandleChannel ch : conn.channels) spare += PooledCandleChannel.CAPACITY - ch.occupancy();
            spare += (PooledCandleConnection.MAX_CHANNELS - conn.channels.size()) * PooledCandleChannel.CAPACITY;
        }
        return spare;
    }

    private Optional<PooledCandleChannel> openChannel(PooledCandleConnection conn, DxLinkClient.CandleCallback mergeListener) {
        try {
            boolean firstChannel = conn.channels.isEmpty();
            List<DxLinkClient.DxLinkChannel> opened = channelOpener.open(conn.client, 1);
            if (opened.isEmpty()) return Optional.empty();
            DxLinkClient.DxLinkChannel raw = opened.get(0);
            if (firstChannel) raw.addCandleListener(mergeListener);
            PooledCandleChannel pooled = new PooledCandleChannel(raw);
            conn.channels.add(pooled);
            return Optional.of(pooled);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            return Optional.empty();
        }
    }

    private PooledCandleConnection openConnectionWithRetry() {
        PooledCandleConnection conn = openConnection();
        if (conn != null) return conn;
        try { Thread.sleep(CONNECTION_RETRY_BACKOFF_MS); } catch (InterruptedException e) { Thread.currentThread().interrupt(); return null; }
        log.info("Retrying candle pool connection after backoff");
        return openConnection();
    }

    private PooledCandleConnection openConnection() {
        DxLinkClient client = new DxLinkClient();
        client.connect(tastyTradeClient.getDxlinkUrl(), tastyTradeClient.getApiQuoteToken());
        if (!client.isConnected()) {
            log.warn("Candle pool connection failed to authenticate, skipping");
            return null;
        }
        return new PooledCandleConnection(client);
    }
}
