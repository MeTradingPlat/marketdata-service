package com.metradingplat.marketdata.domain.usecases;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

import java.util.List;
import java.util.Map;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.models.Quote;

@ExtendWith(MockitoExtension.class)
class GestionarQuoteCUAdapterTest {

    @Mock
    private GestionarComunicacionExternalGatewayIntPort gateway;

    private GestionarQuoteCUAdapter adapter;

    @BeforeEach
    void setUp() {
        adapter = new GestionarQuoteCUAdapter(gateway);
    }

    @Test
    void obtenerQuote_returnsEmptyQuote_whenDataIsNull() {
        when(gateway.getMarketDataByType("AAPL")).thenReturn(null);

        Quote result = adapter.obtenerQuote("AAPL");

        assertEquals("AAPL", result.getSymbol());
        assertNull(result.getBid());
    }

    @Test
    void obtenerQuote_returnsEmptyQuote_whenDataIsEmpty() {
        when(gateway.getMarketDataByType("AAPL")).thenReturn(Map.of());

        Quote result = adapter.obtenerQuote("AAPL");

        assertEquals("AAPL", result.getSymbol());
        assertNull(result.getBid());
    }

    @Test
    void obtenerQuote_returnsEmptyQuote_whenItemsIsNull() {
        Map<String, Object> data = Map.of("other", "value");
        when(gateway.getMarketDataByType("AAPL")).thenReturn(data);

        Quote result = adapter.obtenerQuote("AAPL");

        assertEquals("AAPL", result.getSymbol());
    }

    @Test
    void obtenerQuote_parsesNumericValues() {
        Map<String, Object> item = Map.of(
                "bid", 150.5,
                "ask", 151.0,
                "last", 150.75,
                "open", 149.0,
                "dayHighPrice", 152.0,
                "dayLowPrice", 148.0,
                "close", 150.0,
                "prevClose", 149.5,
                "volume", 1000000.0);

        Map<String, Object> data = Map.of("items", List.of(item));
        when(gateway.getMarketDataByType("AAPL")).thenReturn(data);

        Quote result = adapter.obtenerQuote("AAPL");

        assertEquals("AAPL", result.getSymbol());
        assertEquals(150.5, result.getBid());
        assertEquals(151.0, result.getAsk());
        assertEquals(150.75, result.getLast());
        assertEquals(149.0, result.getOpen());
        assertEquals(152.0, result.getHigh());
        assertEquals(148.0, result.getLow());
        assertEquals(150.0, result.getClose());
        assertEquals(149.5, result.getPrevClose());
        assertEquals(1000000.0, result.getVolume());
    }

    @Test
    void obtenerQuote_parsesStringValues() {
        Map<String, Object> item = Map.of(
                "bid", "150.5",
                "ask", "151.0",
                "last", "150.75");

        Map<String, Object> data = Map.of("items", List.of(item));
        when(gateway.getMarketDataByType("AAPL")).thenReturn(data);

        Quote result = adapter.obtenerQuote("AAPL");

        assertEquals(150.5, result.getBid());
        assertEquals(151.0, result.getAsk());
        assertEquals(150.75, result.getLast());
    }

    @Test
    void obtenerQuote_handlesNullFieldsGracefully() {
        // Use HashMap to allow null values (Map.of doesn't allow nulls)
        java.util.HashMap<String, Object> item = new java.util.HashMap<>();
        item.put("bid", null);
        item.put("ask", null);
        item.put("tradingHalted", null);
        item.put("tradingHaltedReason", null);

        Map<String, Object> data = Map.of("items", List.of(item));
        when(gateway.getMarketDataByType("AAPL")).thenReturn(data);

        Quote result = adapter.obtenerQuote("AAPL");

        assertEquals(0.0, result.getBid());
        assertEquals(0.0, result.getAsk());
        assertFalse(result.getTradingHalted());
        assertNull(result.getTradingHaltedReason());
    }
}
