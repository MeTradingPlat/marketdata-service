package com.metradingplat.marketdata.infrastructure.input.controller;

import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.*;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.*;

import java.time.Instant;
import java.util.List;
import java.util.Map;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.context.annotation.ComponentScan;
import org.springframework.context.annotation.FilterType;
import org.springframework.http.MediaType;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import com.metradingplat.marketdata.application.input.GestionarHistoricalDataCUIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.DTOAnswer.CandleDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.controller.HistoricalDataRestController;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.mapper.HistoricalDataMapper;
import com.metradingplat.marketdata.infrastructure.input.filter.GatewayHeaderFilter;

@WebMvcTest(
    controllers = HistoricalDataRestController.class,
    excludeFilters = @ComponentScan.Filter(type = FilterType.ASSIGNABLE_TYPE, classes = GatewayHeaderFilter.class)
)
class HistoricalDataRestControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @MockitoBean
    private GestionarHistoricalDataCUIntPort useCase;

    @MockitoBean
    private HistoricalDataMapper mapper;

    private static final Instant FIXED_TIMESTAMP = Instant.parse("2025-01-15T10:00:00Z");

    @BeforeEach
    void setUpMapper() {
        // Configure mapper mock to do simple pass-through
        when(mapper.deDominioARespuesta(any(Candle.class))).thenAnswer(inv -> {
            Candle c = inv.getArgument(0);
            return CandleDTORespuesta.builder()
                    .symbol(c.getSymbol()).timestamp(c.getTimestamp())
                    .open(c.getOpen()).high(c.getHigh()).low(c.getLow())
                    .close(c.getClose()).volume(c.getVolume()).build();
        });
        when(mapper.deDominioARespuestas(anyList())).thenAnswer(inv -> {
            List<Candle> candles = inv.getArgument(0);
            return candles.stream().map(c -> CandleDTORespuesta.builder()
                    .symbol(c.getSymbol()).timestamp(c.getTimestamp())
                    .open(c.getOpen()).high(c.getHigh()).low(c.getLow())
                    .close(c.getClose()).volume(c.getVolume()).build()).toList();
        });
    }

    @Test
    void getCandles_returnsOkWithCandles() throws Exception {
        Candle candle = buildCandle("AAPL");
        when(useCase.getCandles(eq("AAPL"), eq(EnumTimeframe.M5), isNull(), isNull()))
                .thenReturn(List.of(candle));

        mockMvc.perform(get("/marketdata/historical/AAPL").param("timeframe", "M5"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].symbol").value("AAPL"))
                .andExpect(jsonPath("$[0].close").value(151.0));
    }

    @Test
    void getCandles_returnsEmptyList() throws Exception {
        when(useCase.getCandles(eq("AAPL"), eq(EnumTimeframe.M5), isNull(), isNull()))
                .thenReturn(List.of());

        mockMvc.perform(get("/marketdata/historical/AAPL").param("timeframe", "M5"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$").isEmpty());
    }

    @Test
    void getLastCandle_returnsNoContent_whenNull() throws Exception {
        when(useCase.getLastCandle("AAPL", EnumTimeframe.M5)).thenReturn(null);

        mockMvc.perform(get("/marketdata/historical/AAPL/last").param("timeframe", "M5"))
                .andExpect(status().isNoContent());
    }

    @Test
    void getCurrentCandle_returnsOk() throws Exception {
        Candle candle = buildCandle("AAPL");
        when(useCase.getCurrentCandle("AAPL", EnumTimeframe.M5)).thenReturn(candle);

        mockMvc.perform(get("/marketdata/historical/AAPL/current").param("timeframe", "M5"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.symbol").value("AAPL"));
    }

    @Test
    void batch_returnsOkWithMultipleSymbols() throws Exception {
        Candle candle = buildCandle("AAPL");
        when(useCase.getCandlesBatch(anyList(), eq(EnumTimeframe.M5), eq(100)))
                .thenReturn(Map.of("AAPL", List.of(candle)));

        mockMvc.perform(post("/marketdata/historical/batch")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"symbols\":[\"AAPL\"],\"timeframe\":\"M5\",\"bars\":100}"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.candlesPorSimbolo.AAPL[0].symbol").value("AAPL"));
    }

    @Test
    void batchLast_returnsOk() throws Exception {
        Candle candle = buildCandle("AAPL");
        when(useCase.getLastCandleBatch(anyList(), eq(EnumTimeframe.M5)))
                .thenReturn(Map.of("AAPL", candle));

        mockMvc.perform(post("/marketdata/historical/batch/last")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"symbols\":[\"AAPL\"],\"timeframe\":\"M5\"}"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.candlePorSimbolo.AAPL.symbol").value("AAPL"));
    }

    private Candle buildCandle(String symbol) {
        return Candle.builder()
                .symbol(symbol).timeframe(EnumTimeframe.M5).timestamp(FIXED_TIMESTAMP)
                .open(150.0).high(152.0).low(149.0).close(151.0).volume(1000.0)
                .build();
    }
}
