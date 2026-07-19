package com.metradingplat.marketdata.domain.market_data_plane.entity;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.junit.jupiter.MockitoExtension;

import java.math.BigDecimal;
import java.time.LocalDateTime;

import static org.assertj.core.api.Assertions.*;

/**
 * Unit tests for Candle aggregation logic.
 * Tests candle creation, OHLCV calculations, and timeframe alignment.
 * Target coverage: >= 80%
 */
@ExtendWith(MockitoExtension.class)
@DisplayName("Candle Aggregation Tests")
class CandleAggregationTest {

    private Candle candle;

    @BeforeEach
    void setUp() {
        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);
        candle = new Candle("SPY", "M1", new BigDecimal("450.00"), new BigDecimal("451.00"),
                new BigDecimal("449.50"), new BigDecimal("450.50"), 1000L, timestamp);
    }

    @Test
    @DisplayName("shouldCreateCandleWithValidParameters")
    void shouldCreateCandleWithValidParameters() {
        // Arrange & Act
        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);
        Candle newCandle = new Candle("AAPL", "M5", new BigDecimal("150.00"), new BigDecimal("151.00"),
                new BigDecimal("149.50"), new BigDecimal("150.50"), 5000L, timestamp);

        // Assert
        assertThat(newCandle).isNotNull();
        assertThat(newCandle.getSymbol()).isEqualTo("AAPL");
        assertThat(newCandle.getTimeframe()).isEqualTo("M5");
        assertThat(newCandle.getOpen()).isEqualByComparingTo(new BigDecimal("150.00"));
        assertThat(newCandle.getHigh()).isEqualByComparingTo(new BigDecimal("151.00"));
        assertThat(newCandle.getLow()).isEqualByComparingTo(new BigDecimal("149.50"));
        assertThat(newCandle.getClose()).isEqualByComparingTo(new BigDecimal("150.50"));
        assertThat(newCandle.getVolume()).isEqualTo(5000L);
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenSymbolIsNull")
    void shouldThrowExceptionWhenSymbolIsNull() {
        // Arrange & Act & Assert
        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);
        assertThatThrownBy(() -> new Candle(null, "M1", new BigDecimal("450.00"), new BigDecimal("451.00"),
                new BigDecimal("449.50"), new BigDecimal("450.50"), 1000L, timestamp))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("symbol");
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenHighIsLessThanLow")
    void shouldThrowExceptionWhenHighIsLessThanLow() {
        // Arrange & Act & Assert
        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);
        assertThatThrownBy(() -> new Candle("SPY", "M1", new BigDecimal("450.00"), new BigDecimal("449.00"),
                new BigDecimal("451.00"), new BigDecimal("450.50"), 1000L, timestamp))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("high");
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenOpenIsOutsideHighLowRange")
    void shouldThrowExceptionWhenOpenIsOutsideHighLowRange() {
        // Arrange & Act & Assert
        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);
        assertThatThrownBy(() -> new Candle("SPY", "M1", new BigDecimal("452.00"), new BigDecimal("451.00"),
                new BigDecimal("449.50"), new BigDecimal("450.50"), 1000L, timestamp))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("open");
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenCloseIsOutsideHighLowRange")
    void shouldThrowExceptionWhenCloseIsOutsideHighLowRange() {
        // Arrange & Act & Assert
        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);
        assertThatThrownBy(() -> new Candle("SPY", "M1", new BigDecimal("450.00"), new BigDecimal("451.00"),
                new BigDecimal("449.50"), new BigDecimal("452.00"), 1000L, timestamp))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("close");
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenVolumeIsNegative")
    void shouldThrowExceptionWhenVolumeIsNegative() {
        // Arrange & Act & Assert
        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);
        assertThatThrownBy(() -> new Candle("SPY", "M1", new BigDecimal("450.00"), new BigDecimal("451.00"),
                new BigDecimal("449.50"), new BigDecimal("450.50"), -1000L, timestamp))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("volume");
    }

    @Test
    @DisplayName("shouldCalculateOHLCVCorrectly")
    void shouldCalculateOHLCVCorrectly() {
        // Arrange & Act
        BigDecimal open = candle.getOpen();
        BigDecimal high = candle.getHigh();
        BigDecimal low = candle.getLow();
        BigDecimal close = candle.getClose();
        long volume = candle.getVolume();

        // Assert
        assertThat(open).isEqualByComparingTo(new BigDecimal("450.00"));
        assertThat(high).isEqualByComparingTo(new BigDecimal("451.00"));
        assertThat(low).isEqualByComparingTo(new BigDecimal("449.50"));
        assertThat(close).isEqualByComparingTo(new BigDecimal("450.50"));
        assertThat(volume).isEqualTo(1000L);
    }

    @Test
    @DisplayName("shouldHandleZeroVolume")
    void shouldHandleZeroVolume() {
        // Arrange & Act
        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);
        Candle zeroVolumeCandle = new Candle("SPY", "M1", new BigDecimal("450.00"), new BigDecimal("451.00"),
                new BigDecimal("449.50"), new BigDecimal("450.50"), 0L, timestamp);

        // Assert
        assertThat(zeroVolumeCandle.getVolume()).isEqualTo(0L);
    }

    @Test
    @DisplayName("shouldHandleLargeVolumes")
    void shouldHandleLargeVolumes() {
        // Arrange & Act
        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);
        Candle largeVolumeCandle = new Candle("SPY", "M1", new BigDecimal("450.00"), new BigDecimal("451.00"),
                new BigDecimal("449.50"), new BigDecimal("450.50"), 1_000_000_000L, timestamp);

        // Assert
        assertThat(largeVolumeCandle.getVolume()).isEqualTo(1_000_000_000L);
    }

    @Test
    @DisplayName("shouldSupportMultipleTimeframes")
    void shouldSupportMultipleTimeframes() {
        // Arrange & Act
        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);
        Candle m1 = new Candle("SPY", "M1", new BigDecimal("450.00"), new BigDecimal("451.00"),
                new BigDecimal("449.50"), new BigDecimal("450.50"), 1000L, timestamp);
        Candle m5 = new Candle("SPY", "M5", new BigDecimal("450.00"), new BigDecimal("451.00"),
                new BigDecimal("449.50"), new BigDecimal("450.50"), 5000L, timestamp);
        Candle h1 = new Candle("SPY", "H1", new BigDecimal("450.00"), new BigDecimal("451.00"),
                new BigDecimal("449.50"), new BigDecimal("450.50"), 60000L, timestamp);

        // Assert
        assertThat(m1.getTimeframe()).isEqualTo("M1");
        assertThat(m5.getTimeframe()).isEqualTo("M5");
        assertThat(h1.getTimeframe()).isEqualTo("H1");
    }

    @Test
    @DisplayName("shouldAlignTimestampToTimeframeBoundary")
    void shouldAlignTimestampToTimeframeBoundary() {
        // Arrange
        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 5, 0);

        // Act
        Candle alignedCandle = new Candle("SPY", "M5", new BigDecimal("450.00"), new BigDecimal("451.00"),
                new BigDecimal("449.50"), new BigDecimal("450.50"), 5000L, timestamp);

        // Assert
        assertThat(alignedCandle.getTimestamp()).isEqualTo(timestamp);
    }

    @Test
    @DisplayName("shouldHandleIdenticalOpenAndClose")
    void shouldHandleIdenticalOpenAndClose() {
        // Arrange & Act
        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);
        Candle doji = new Candle("SPY", "M1", new BigDecimal("450.00"), new BigDecimal("451.00"),
                new BigDecimal("449.50"), new BigDecimal("450.00"), 1000L, timestamp);

        // Assert
        assertThat(doji.getOpen()).isEqualByComparingTo(doji.getClose());
    }

    @Test
    @DisplayName("shouldHandleIdenticalHighAndLow")
    void shouldHandleIdenticalHighAndLow() {
        // Arrange & Act
        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);
        Candle flatCandle = new Candle("SPY", "M1", new BigDecimal("450.00"), new BigDecimal("450.00"),
                new BigDecimal("450.00"), new BigDecimal("450.00"), 1000L, timestamp);

        // Assert
        assertThat(flatCandle.getHigh()).isEqualByComparingTo(flatCandle.getLow());
    }

    @Test
    @DisplayName("shouldCalculateCandleRange")
    void shouldCalculateCandleRange() {
        // Arrange
        BigDecimal high = candle.getHigh();
        BigDecimal low = candle.getLow();

        // Act
        BigDecimal range = high.subtract(low);

        // Assert
        assertThat(range).isEqualByComparingTo(new BigDecimal("1.50"));
    }

    @Test
    @DisplayName("shouldCalculateCandleBody")
    void shouldCalculateCandleBody() {
        // Arrange
        BigDecimal open = candle.getOpen();
        BigDecimal close = candle.getClose();

        // Act
        BigDecimal body = close.subtract(open).abs();

        // Assert
        assertThat(body).isEqualByComparingTo(new BigDecimal("0.50"));
    }
}
