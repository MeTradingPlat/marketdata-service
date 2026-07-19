package com.metradingplat.marketdata.adapter.external.dxlink;

import com.metradingplat.marketdata.domain.market_data_plane.entity.Candle;
import com.metradingplat.marketdata.domain.market_data_plane.entity.Greeks;
import com.metradingplat.marketdata.domain.market_data_plane.entity.Quote;
import com.metradingplat.marketdata.domain.market_data_plane.entity.Trade;
import com.metradingplat.marketdata.domain.market_data_plane.entity.Underlying;
import com.metradingplat.marketdata.domain.market_data_plane.port.MarketDataPublisherPort;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.math.BigDecimal;

import static org.assertj.core.api.Assertions.assertThatCode;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.doThrow;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.verifyNoMoreInteractions;

/**
 * Unit tests for DxLinkMarketDataListener.
 * Verifies that parsed market data events are correctly published to the output port.
 */
@ExtendWith(MockitoExtension.class)
@DisplayName("DxLinkMarketDataListener Tests")
class DxLinkMarketDataListenerTest {

    @Mock
    private MarketDataPublisherPort marketDataPublisher;

    private DxLinkMarketDataListener listener;

    @BeforeEach
    void setUp() {
        listener = new DxLinkMarketDataListener(marketDataPublisher);
    }

    @Test
    @DisplayName("shouldPublishQuoteWhenHandleQuoteIsCalledWithValidQuote")
    void shouldPublishQuoteWhenHandleQuoteIsCalledWithValidQuote() {
        // Arrange
        Quote quote = new Quote("SPY",
            new BigDecimal("450.10"), new BigDecimal("450.15"), new BigDecimal("450.12"));

        // Act
        listener.handleQuote(quote);

        // Assert
        verify(marketDataPublisher).publishQuote(quote);
        verifyNoMoreInteractions(marketDataPublisher);
    }

    @Test
    @DisplayName("shouldPublishTradeWhenHandleTradeIsCalledWithValidTrade")
    void shouldPublishTradeWhenHandleTradeIsCalledWithValidTrade() {
        // Arrange
        Trade trade = new Trade("AAPL", new BigDecimal("150.27"), 1000L, Trade.TradeSize.BUY);

        // Act
        listener.handleTrade(trade);

        // Assert
        verify(marketDataPublisher).publishTrade(trade);
        verifyNoMoreInteractions(marketDataPublisher);
    }

    @Test
    @DisplayName("shouldPublishCandleWhenHandleCandleIsCalledWithValidCandle")
    void shouldPublishCandleWhenHandleCandleIsCalledWithValidCandle() {
        // Arrange
        Candle candle = new Candle("SPY", Candle.Timeframe.M1);
        candle.setClose(new BigDecimal("450.75"));

        // Act
        listener.handleCandle(candle);

        // Assert
        verify(marketDataPublisher).publishCandle(candle);
        verifyNoMoreInteractions(marketDataPublisher);
    }

    @Test
    @DisplayName("shouldPublishGreeksWhenHandleGreeksIsCalledWithValidGreeks")
    void shouldPublishGreeksWhenHandleGreeksIsCalledWithValidGreeks() {
        // Arrange
        Greeks greeks = new Greeks(".SPY240119C450");
        greeks.setDelta(new BigDecimal("0.75"));
        greeks.setGamma(new BigDecimal("0.02"));

        // Act
        listener.handleGreeks(greeks);

        // Assert
        verify(marketDataPublisher).publishGreeks(greeks);
        verifyNoMoreInteractions(marketDataPublisher);
    }

    @Test
    @DisplayName("shouldPublishUnderlyingWhenHandleUnderlyingIsCalledWithValidUnderlying")
    void shouldPublishUnderlyingWhenHandleUnderlyingIsCalledWithValidUnderlying() {
        // Arrange
        Underlying underlying = new Underlying("SPY");
        underlying.setBid(new BigDecimal("450.10"));
        underlying.setAsk(new BigDecimal("450.15"));

        // Act
        listener.handleUnderlying(underlying);

        // Assert
        verify(marketDataPublisher).publishUnderlying(underlying);
        verifyNoMoreInteractions(marketDataPublisher);
    }

    @Test
    @DisplayName("shouldPublishAllFiveEventTypesIndependentlyWhenEachHandlerIsCalled")
    void shouldPublishAllFiveEventTypesIndependentlyWhenEachHandlerIsCalled() {
        // Arrange
        Quote quote = new Quote("SPY", BigDecimal.TEN, BigDecimal.TEN, BigDecimal.TEN);
        Trade trade = new Trade("SPY", BigDecimal.TEN, 100L, Trade.TradeSize.SELL);
        Candle candle = new Candle("SPY", Candle.Timeframe.H1);
        Greeks greeks = new Greeks(".SPY240119C450");
        Underlying underlying = new Underlying("SPY");

        // Act
        listener.handleQuote(quote);
        listener.handleTrade(trade);
        listener.handleCandle(candle);
        listener.handleGreeks(greeks);
        listener.handleUnderlying(underlying);

        // Assert — cada tipo se publica exactamente una vez
        verify(marketDataPublisher).publishQuote(quote);
        verify(marketDataPublisher).publishTrade(trade);
        verify(marketDataPublisher).publishCandle(candle);
        verify(marketDataPublisher).publishGreeks(greeks);
        verify(marketDataPublisher).publishUnderlying(underlying);
    }

    @Test
    @DisplayName("shouldNotPropagateExceptionWhenPublisherThrowsOnQuote")
    void shouldNotPropagateExceptionWhenPublisherThrowsOnQuote() {
        // Arrange — simular fallo de Kafka/ZeroMQ
        Quote quote = new Quote("SPY", BigDecimal.TEN, BigDecimal.TEN, BigDecimal.TEN);
        doThrow(new RuntimeException("Kafka unavailable"))
            .when(marketDataPublisher).publishQuote(any());

        // Act & Assert — el listener no debe romper el flujo de eventos
        assertThatCode(() -> listener.handleQuote(quote)).doesNotThrowAnyException();
    }

    @Test
    @DisplayName("shouldNotPropagateExceptionWhenPublisherThrowsOnGreeks")
    void shouldNotPropagateExceptionWhenPublisherThrowsOnGreeks() {
        // Arrange
        Greeks greeks = new Greeks(".SPY240119C450");
        doThrow(new RuntimeException("ZeroMQ socket closed"))
            .when(marketDataPublisher).publishGreeks(any());

        // Act & Assert
        assertThatCode(() -> listener.handleGreeks(greeks)).doesNotThrowAnyException();
    }
}
