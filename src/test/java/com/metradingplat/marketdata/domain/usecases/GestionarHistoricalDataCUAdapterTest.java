package com.metradingplat.marketdata.domain.usecases;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

import java.time.Duration;
import java.time.Instant;
import java.time.OffsetDateTime;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;

@ExtendWith(MockitoExtension.class)
class GestionarHistoricalDataCUAdapterTest {

    @Mock
    private GestionarComunicacionExternalGatewayIntPort gateway;

    private GestionarHistoricalDataCUAdapter adapter;

    @BeforeEach
    void setUp() {
        adapter = new GestionarHistoricalDataCUAdapter(gateway);
    }

    // --- getCandles ---

    @Test
    void getCandles_returnsEmptyList_whenGatewayReturnsNull() {
        when(gateway.getCandles("AAPL", EnumTimeframe.M5)).thenReturn(null);

        List<Candle> result = adapter.getCandles("AAPL", EnumTimeframe.M5, null, null);

        assertTrue(result.isEmpty());
    }

    @Test
    void getCandles_returnsEmptyList_whenGatewayReturnsEmpty() {
        when(gateway.getCandles("AAPL", EnumTimeframe.M5)).thenReturn(List.of());

        List<Candle> result = adapter.getCandles("AAPL", EnumTimeframe.M5, null, null);

        assertTrue(result.isEmpty());
    }

    @Test
    void getCandles_filtersOutIncompleteBars() {
        Instant now = Instant.now();
        // Completed bar: started 10 minutes ago, 5min duration = ended 5 min ago
        Candle completed = buildCandle("AAPL", now.minus(Duration.ofMinutes(10)), 100.0);
        // Incomplete bar: started 2 minutes ago, 5min duration = ends in 3 min
        Candle incomplete = buildCandle("AAPL", now.minus(Duration.ofMinutes(2)), 101.0);

        when(gateway.getCandles("AAPL", EnumTimeframe.M5)).thenReturn(List.of(completed, incomplete));

        List<Candle> result = adapter.getCandles("AAPL", EnumTimeframe.M5, null, null);

        assertEquals(1, result.size());
        assertEquals(100.0, result.get(0).getClose());
    }

    @Test
    void getCandles_filtersBarsByEndDate() {
        Instant now = Instant.now();
        OffsetDateTime endDate = OffsetDateTime.ofInstant(now.minus(Duration.ofMinutes(30)), ZoneOffset.UTC);

        // Bar completed well before endDate
        Candle earlyBar = buildCandle("AAPL", now.minus(Duration.ofMinutes(60)), 100.0);
        // Bar completed after endDate
        Candle lateBar = buildCandle("AAPL", now.minus(Duration.ofMinutes(20)), 102.0);

        when(gateway.getCandles("AAPL", EnumTimeframe.M5)).thenReturn(List.of(earlyBar, lateBar));

        List<Candle> result = adapter.getCandles("AAPL", EnumTimeframe.M5, endDate, null);

        assertEquals(1, result.size());
        assertEquals(100.0, result.get(0).getClose());
    }

    @Test
    void getCandles_truncatesToNBars() {
        Instant now = Instant.now();
        List<Candle> candles = new ArrayList<>();
        for (int i = 10; i >= 1; i--) {
            candles.add(buildCandle("AAPL", now.minus(Duration.ofMinutes(i * 5 + 5)), 100.0 + i));
        }

        when(gateway.getCandles("AAPL", EnumTimeframe.M5)).thenReturn(candles);

        List<Candle> result = adapter.getCandles("AAPL", EnumTimeframe.M5, null, 3);

        assertEquals(3, result.size());
    }

    @Test
    void getCandles_returnsAllWhenBarsGreaterThanAvailable() {
        Instant now = Instant.now();
        List<Candle> candles = List.of(
                buildCandle("AAPL", now.minus(Duration.ofMinutes(15)), 100.0),
                buildCandle("AAPL", now.minus(Duration.ofMinutes(10)), 101.0));

        when(gateway.getCandles("AAPL", EnumTimeframe.M5)).thenReturn(candles);

        List<Candle> result = adapter.getCandles("AAPL", EnumTimeframe.M5, null, 50);

        assertEquals(2, result.size());
    }

    // --- getCandlesBatch ---

    @Test
    void getCandlesBatch_filtersAndTruncatesPerSymbol() {
        Instant now = Instant.now();
        List<Candle> aaplCandles = new ArrayList<>();
        for (int i = 5; i >= 1; i--) {
            aaplCandles.add(buildCandle("AAPL", now.minus(Duration.ofMinutes(i * 5 + 5)), 100.0 + i));
        }
        List<Candle> googlCandles = List.of(
                buildCandle("GOOGL", now.minus(Duration.ofMinutes(15)), 200.0));

        Map<String, List<Candle>> rawData = Map.of("AAPL", aaplCandles, "GOOGL", googlCandles);
        when(gateway.getCandlesBatch(List.of("AAPL", "GOOGL"), EnumTimeframe.M5, 2)).thenReturn(rawData);

        Map<String, List<Candle>> result = adapter.getCandlesBatch(List.of("AAPL", "GOOGL"), EnumTimeframe.M5, 2);

        assertEquals(2, result.get("AAPL").size());
        assertEquals(1, result.get("GOOGL").size());
    }

    @Test
    void getCandlesBatch_handlesEmptyListForSymbol() {
        Map<String, List<Candle>> rawData = Map.of("AAPL", List.of());
        when(gateway.getCandlesBatch(List.of("AAPL"), EnumTimeframe.M5, 10)).thenReturn(rawData);

        Map<String, List<Candle>> result = adapter.getCandlesBatch(List.of("AAPL"), EnumTimeframe.M5, 10);

        assertTrue(result.get("AAPL").isEmpty());
    }

    // --- getLastCandle ---

    @Test
    void getLastCandle_returnsLastCompletedCandle() {
        Instant now = Instant.now();
        Candle completed = buildCandle("AAPL", now.minus(Duration.ofMinutes(10)), 100.0);
        Candle incomplete = buildCandle("AAPL", now.minus(Duration.ofMinutes(2)), 101.0);

        when(gateway.getCandles("AAPL", EnumTimeframe.M5)).thenReturn(List.of(completed, incomplete));

        Candle result = adapter.getLastCandle("AAPL", EnumTimeframe.M5);

        assertNotNull(result);
        assertEquals(100.0, result.getClose());
    }

    @Test
    void getLastCandle_returnsNull_whenNoCandles() {
        when(gateway.getCandles("AAPL", EnumTimeframe.M5)).thenReturn(List.of());

        Candle result = adapter.getLastCandle("AAPL", EnumTimeframe.M5);

        assertNull(result);
    }

    // --- getCurrentCandle ---

    @Test
    void getCurrentCandle_returnsIncompleteBar() {
        Instant now = Instant.now();
        Candle completed = buildCandle("AAPL", now.minus(Duration.ofMinutes(10)), 100.0);
        Candle incomplete = buildCandle("AAPL", now.minus(Duration.ofMinutes(2)), 101.0);

        when(gateway.getCandles("AAPL", EnumTimeframe.M5)).thenReturn(List.of(completed, incomplete));

        Candle result = adapter.getCurrentCandle("AAPL", EnumTimeframe.M5);

        assertNotNull(result);
        assertEquals(101.0, result.getClose());
    }

    @Test
    void getCurrentCandle_returnsNull_whenAllBarsCompleted() {
        Instant now = Instant.now();
        Candle completed = buildCandle("AAPL", now.minus(Duration.ofMinutes(10)), 100.0);

        when(gateway.getCandles("AAPL", EnumTimeframe.M5)).thenReturn(List.of(completed));

        Candle result = adapter.getCurrentCandle("AAPL", EnumTimeframe.M5);

        assertNull(result);
    }

    @Test
    void getCurrentCandle_returnsNull_whenNoCandles() {
        when(gateway.getCandles("AAPL", EnumTimeframe.M5)).thenReturn(null);

        Candle result = adapter.getCurrentCandle("AAPL", EnumTimeframe.M5);

        assertNull(result);
    }

    // --- helper ---

    private Candle buildCandle(String symbol, Instant timestamp, Double close) {
        return Candle.builder()
                .symbol(symbol)
                .timeframe(EnumTimeframe.M5)
                .timestamp(timestamp)
                .open(close - 1)
                .high(close + 1)
                .low(close - 2)
                .close(close)
                .volume(1000.0)
                .build();
    }
}
