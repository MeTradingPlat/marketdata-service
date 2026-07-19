package com.metradingplat.marketdata.adapter.external.dxlink;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.metradingplat.marketdata.domain.market_data_plane.entity.Candle;
import com.metradingplat.marketdata.domain.market_data_plane.entity.Greeks;
import com.metradingplat.marketdata.domain.market_data_plane.entity.Quote;
import com.metradingplat.marketdata.domain.market_data_plane.entity.Trade;
import com.metradingplat.marketdata.domain.market_data_plane.entity.Underlying;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import java.math.BigDecimal;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatCode;

/**
 * Unit tests for DxLinkEventListener.
 * Verifies correct parsing of all 5 market data event types from dxLink WebSocket.
 */
@DisplayName("DxLinkEventListener Tests")
class DxLinkEventListenerTest {

    /** Concrete testable implementation of the abstract base class. */
    private static class TestableEventListener extends DxLinkEventListener {
        Quote lastQuote;
        Trade lastTrade;
        Candle lastCandle;
        Greeks lastGreeks;
        Underlying lastUnderlying;

        @Override protected void handleQuote(Quote q)           { this.lastQuote = q; }
        @Override protected void handleTrade(Trade t)           { this.lastTrade = t; }
        @Override protected void handleCandle(Candle c)         { this.lastCandle = c; }
        @Override protected void handleGreeks(Greeks g)         { this.lastGreeks = g; }
        @Override protected void handleUnderlying(Underlying u) { this.lastUnderlying = u; }
    }

    private final ObjectMapper mapper = new ObjectMapper();
    private TestableEventListener listener;

    @BeforeEach
    void setUp() {
        listener = new TestableEventListener();
    }

    // ─── Quote ───────────────────────────────────────────────────────────────

    @Test
    @DisplayName("shouldParseQuoteEventWithCorrectBidAskLastWhenJsonIsValid")
    void shouldParseQuoteEventWithCorrectBidAskLastWhenJsonIsValid() throws Exception {
        // Arrange
        JsonNode event = mapper.readTree(
            "{\"type\":\"Quote\",\"symbol\":\"SPY\","
            + "\"bid\":450.10,\"ask\":450.15,\"last\":450.12}");

        // Act
        listener.onQuote(event);

        // Assert
        assertThat(listener.lastQuote).isNotNull();
        assertThat(listener.lastQuote.getSymbol()).isEqualTo("SPY");
        assertThat(listener.lastQuote.getBid()).isEqualByComparingTo("450.1");
        assertThat(listener.lastQuote.getAsk()).isEqualByComparingTo("450.15");
        assertThat(listener.lastQuote.getLast()).isEqualByComparingTo("450.12");
        assertThat(listener.lastQuote.getTimestamp()).isNotNull();
    }

    @Test
    @DisplayName("shouldNotCallHandleQuoteWhenQuoteJsonIsMalformed")
    void shouldNotCallHandleQuoteWhenQuoteJsonIsMalformed() throws Exception {
        // Arrange — falta el campo "bid"
        JsonNode malformed = mapper.readTree("{\"type\":\"Quote\",\"symbol\":\"SPY\"}");

        // Act & Assert — no debe propagar excepción ni llamar al handler
        assertThatCode(() -> listener.onQuote(malformed)).doesNotThrowAnyException();
        assertThat(listener.lastQuote).isNull();
    }

    // ─── Trade ───────────────────────────────────────────────────────────────

    @Test
    @DisplayName("shouldParseTradeEventWithCorrectPriceSizeSideWhenJsonIsValid")
    void shouldParseTradeEventWithCorrectPriceSizeSideWhenJsonIsValid() throws Exception {
        // Arrange
        JsonNode event = mapper.readTree(
            "{\"type\":\"Trade\",\"symbol\":\"AAPL\","
            + "\"price\":150.27,\"size\":1000,\"side\":\"BUY\"}");

        // Act
        listener.onTrade(event);

        // Assert
        assertThat(listener.lastTrade).isNotNull();
        assertThat(listener.lastTrade.getSymbol()).isEqualTo("AAPL");
        assertThat(listener.lastTrade.getPrice()).isEqualByComparingTo("150.27");
        assertThat(listener.lastTrade.getSize()).isEqualTo(1000L);
        assertThat(listener.lastTrade.getSide()).isEqualTo(Trade.TradeSize.BUY);
    }

    @Test
    @DisplayName("shouldNotCallHandleTradeWhenTradeJsonHasInvalidSide")
    void shouldNotCallHandleTradeWhenTradeJsonHasInvalidSide() throws Exception {
        // Arrange — "side" con valor inválido
        JsonNode malformed = mapper.readTree(
            "{\"type\":\"Trade\",\"symbol\":\"AAPL\","
            + "\"price\":150.27,\"size\":1000,\"side\":\"INVALID\"}");

        // Act & Assert
        assertThatCode(() -> listener.onTrade(malformed)).doesNotThrowAnyException();
        assertThat(listener.lastTrade).isNull();
    }

    // ─── Candle ──────────────────────────────────────────────────────────────

    @Test
    @DisplayName("shouldParseCandleEventWithOhlcvAndTimeframeWhenJsonIsValid")
    void shouldParseCandleEventWithOhlcvAndTimeframeWhenJsonIsValid() throws Exception {
        // Arrange
        JsonNode event = mapper.readTree(
            "{\"type\":\"Candle\",\"symbol\":\"SPY\",\"timeframe\":\"M1\","
            + "\"open\":450.00,\"high\":451.00,\"low\":449.50,"
            + "\"close\":450.75,\"volume\":50000}");

        // Act
        listener.onCandle(event);

        // Assert
        assertThat(listener.lastCandle).isNotNull();
        assertThat(listener.lastCandle.getSymbol()).isEqualTo("SPY");
        assertThat(listener.lastCandle.getTimeframe()).isEqualTo(Candle.Timeframe.M1);
        assertThat(listener.lastCandle.getOpen()).isEqualByComparingTo("450.0");
        assertThat(listener.lastCandle.getHigh()).isEqualByComparingTo("451.0");
        assertThat(listener.lastCandle.getLow()).isEqualByComparingTo("449.5");
        assertThat(listener.lastCandle.getClose()).isEqualByComparingTo("450.75");
        assertThat(listener.lastCandle.getVolume()).isEqualTo(50000L);
    }

    @Test
    @DisplayName("shouldNotCallHandleCandleWhenTimeframeIsUnknown")
    void shouldNotCallHandleCandleWhenTimeframeIsUnknown() throws Exception {
        // Arrange — timeframe inválido
        JsonNode malformed = mapper.readTree(
            "{\"type\":\"Candle\",\"symbol\":\"SPY\",\"timeframe\":\"INVALID\","
            + "\"open\":450.00,\"high\":451.00,\"low\":449.50,\"close\":450.75,\"volume\":50000}");

        // Act & Assert
        assertThatCode(() -> listener.onCandle(malformed)).doesNotThrowAnyException();
        assertThat(listener.lastCandle).isNull();
    }

    // ─── Greeks ──────────────────────────────────────────────────────────────

    @Test
    @DisplayName("shouldParseGreeksEventWithAllFiveGreeksWhenJsonIsValid")
    void shouldParseGreeksEventWithAllFiveGreeksWhenJsonIsValid() throws Exception {
        // Arrange
        JsonNode event = mapper.readTree(
            "{\"type\":\"Greeks\",\"symbol\":\".SPY240119C450\","
            + "\"delta\":0.75,\"gamma\":0.02,\"theta\":-0.05,"
            + "\"vega\":0.10,\"rho\":0.15}");

        // Act
        listener.onGreeks(event);

        // Assert
        assertThat(listener.lastGreeks).isNotNull();
        assertThat(listener.lastGreeks.getSymbol()).isEqualTo(".SPY240119C450");
        assertThat(listener.lastGreeks.getDelta()).isEqualByComparingTo("0.75");
        assertThat(listener.lastGreeks.getGamma()).isEqualByComparingTo("0.02");
        assertThat(listener.lastGreeks.getTheta()).isNegative();
        assertThat(listener.lastGreeks.getVega()).isPositive();
        assertThat(listener.lastGreeks.getRho()).isEqualByComparingTo("0.15");
    }

    @Test
    @DisplayName("shouldNotCallHandleGreeksWhenGreeksJsonIsMalformed")
    void shouldNotCallHandleGreeksWhenGreeksJsonIsMalformed() throws Exception {
        // Arrange — falta "delta"
        JsonNode malformed = mapper.readTree(
            "{\"type\":\"Greeks\",\"symbol\":\".SPY240119C450\"}");

        // Act & Assert
        assertThatCode(() -> listener.onGreeks(malformed)).doesNotThrowAnyException();
        assertThat(listener.lastGreeks).isNull();
    }

    // ─── Underlying ──────────────────────────────────────────────────────────

    @Test
    @DisplayName("shouldParseUnderlyingEventWithBidAskVolumeOpenInterestWhenJsonIsValid")
    void shouldParseUnderlyingEventWithBidAskVolumeOpenInterestWhenJsonIsValid() throws Exception {
        // Arrange
        JsonNode event = mapper.readTree(
            "{\"type\":\"Underlying\",\"symbol\":\"SPY\","
            + "\"bid\":450.10,\"ask\":450.15,\"last\":450.12,"
            + "\"volume\":1000000,\"openInterest\":500000}");

        // Act
        listener.onUnderlying(event);

        // Assert
        assertThat(listener.lastUnderlying).isNotNull();
        assertThat(listener.lastUnderlying.getSymbol()).isEqualTo("SPY");
        assertThat(listener.lastUnderlying.getBid()).isEqualByComparingTo("450.1");
        assertThat(listener.lastUnderlying.getAsk()).isEqualByComparingTo("450.15");
        assertThat(listener.lastUnderlying.getVolume()).isEqualTo(1000000L);
        assertThat(listener.lastUnderlying.getOpenInterest()).isEqualTo(500000L);
    }

    @Test
    @DisplayName("shouldNotCallHandleUnderlyingWhenUnderlyingJsonIsMalformed")
    void shouldNotCallHandleUnderlyingWhenUnderlyingJsonIsMalformed() throws Exception {
        // Arrange — falta "bid"
        JsonNode malformed = mapper.readTree(
            "{\"type\":\"Underlying\",\"symbol\":\"SPY\"}");

        // Act & Assert
        assertThatCode(() -> listener.onUnderlying(malformed)).doesNotThrowAnyException();
        assertThat(listener.lastUnderlying).isNull();
    }
}
