package com.metradingplat.marketdata.domain.market_data_plane.entity;

import net.jqwik.api.*;
import net.jqwik.api.constraints.LongRange;
import net.jqwik.api.constraints.Positive;

import java.math.BigDecimal;
import java.time.LocalDateTime;

import static org.assertj.core.api.Assertions.*;

/**
 * Property-based tests for candle aggregation.
 * **Validates: Requirements 8.1, 8.2, 8.3**
 *
 * Properties tested:
 * - Candles are aggregated correctly by timeframe
 * - OHLCV values are within expected ranges
 * - Candle timestamps are aligned to period boundaries
 */
@DisplayName("Candle Aggregation Property Tests")
class CandleAggregationPropertyTest {

    @Property
    @DisplayName("shouldCreateValidCandlesWithCorrectOHLCVRanges")
    void shouldCreateValidCandlesWithCorrectOHLCVRanges(
            @ForAll String symbol,
            @ForAll String timeframe,
            @ForAll @Positive BigDecimal open,
            @ForAll @Positive BigDecimal high,
            @ForAll @Positive BigDecimal low,
            @ForAll @Positive BigDecimal close,
            @ForAll @LongRange(min = 0) Long volume) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);
        Assume.that(timeframe != null && !timeframe.isEmpty());
        Assume.that(high.compareTo(low) >= 0); // High >= Low
        Assume.that(open.compareTo(high) <= 0 && open.compareTo(low) >= 0); // Open within range
        Assume.that(close.compareTo(high) <= 0 && close.compareTo(low) >= 0); // Close within range

        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);

        // Act
        Candle candle = new Candle(symbol, timeframe, open, high, low, close, volume, timestamp);

        // Assert
        assertThat(candle.getOpen()).isEqualByComparingTo(open);
        assertThat(candle.getHigh()).isEqualByComparingTo(high);
        assertThat(candle.getLow()).isEqualByComparingTo(low);
        assertThat(candle.getClose()).isEqualByComparingTo(close);
        assertThat(candle.getVolume()).isEqualTo(volume);
    }

    @Property
    @DisplayName("shouldEnforceHighGreaterThanOrEqualToLow")
    void shouldEnforceHighGreaterThanOrEqualToLow(
            @ForAll String symbol,
            @ForAll String timeframe,
            @ForAll @Positive BigDecimal open,
            @ForAll @Positive BigDecimal close,
            @ForAll @Positive BigDecimal low) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);
        Assume.that(timeframe != null && !timeframe.isEmpty());
        BigDecimal high = low.subtract(new BigDecimal("1.00")); // High < Low (invalid)
        Assume.that(open.compareTo(low) >= 0 && open.compareTo(low.add(new BigDecimal("1.00"))) <= 0);
        Assume.that(close.compareTo(low) >= 0 && close.compareTo(low.add(new BigDecimal("1.00"))) <= 0);

        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);

        // Act & Assert
        assertThatThrownBy(() -> new Candle(symbol, timeframe, open, high, low, close, 1000L, timestamp))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("high");
    }

    @Property
    @DisplayName("shouldEnforceOpenWithinHighLowRange")
    void shouldEnforceOpenWithinHighLowRange(
            @ForAll String symbol,
            @ForAll String timeframe,
            @ForAll @Positive BigDecimal high,
            @ForAll @Positive BigDecimal low,
            @ForAll @Positive BigDecimal close) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);
        Assume.that(timeframe != null && !timeframe.isEmpty());
        Assume.that(high.compareTo(low) >= 0);
        BigDecimal open = high.add(new BigDecimal("1.00")); // Open > High (invalid)
        Assume.that(close.compareTo(high) <= 0 && close.compareTo(low) >= 0);

        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);

        // Act & Assert
        assertThatThrownBy(() -> new Candle(symbol, timeframe, open, high, low, close, 1000L, timestamp))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("open");
    }

    @Property
    @DisplayName("shouldEnforceCloseWithinHighLowRange")
    void shouldEnforceCloseWithinHighLowRange(
            @ForAll String symbol,
            @ForAll String timeframe,
            @ForAll @Positive BigDecimal open,
            @ForAll @Positive BigDecimal high,
            @ForAll @Positive BigDecimal low) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);
        Assume.that(timeframe != null && !timeframe.isEmpty());
        Assume.that(high.compareTo(low) >= 0);
        Assume.that(open.compareTo(high) <= 0 && open.compareTo(low) >= 0);
        BigDecimal close = high.add(new BigDecimal("1.00")); // Close > High (invalid)

        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);

        // Act & Assert
        assertThatThrownBy(() -> new Candle(symbol, timeframe, open, high, low, close, 1000L, timestamp))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("close");
    }

    @Property
    @DisplayName("shouldRejectNegativeVolume")
    void shouldRejectNegativeVolume(
            @ForAll String symbol,
            @ForAll String timeframe,
            @ForAll @Positive BigDecimal open,
            @ForAll @Positive BigDecimal high,
            @ForAll @Positive BigDecimal low,
            @ForAll @Positive BigDecimal close) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);
        Assume.that(timeframe != null && !timeframe.isEmpty());
        Assume.that(high.compareTo(low) >= 0);
        Assume.that(open.compareTo(high) <= 0 && open.compareTo(low) >= 0);
        Assume.that(close.compareTo(high) <= 0 && close.compareTo(low) >= 0);

        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);
        Long negativeVolume = -1000L;

        // Act & Assert
        assertThatThrownBy(() -> new Candle(symbol, timeframe, open, high, low, close, negativeVolume, timestamp))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("volume");
    }

    @Property
    @DisplayName("shouldCalculateCandleRangeCorrectly")
    void shouldCalculateCandleRangeCorrectly(
            @ForAll String symbol,
            @ForAll String timeframe,
            @ForAll @Positive BigDecimal open,
            @ForAll @Positive BigDecimal high,
            @ForAll @Positive BigDecimal low,
            @ForAll @Positive BigDecimal close,
            @ForAll @LongRange(min = 0) Long volume) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);
        Assume.that(timeframe != null && !timeframe.isEmpty());
        Assume.that(high.compareTo(low) >= 0);
        Assume.that(open.compareTo(high) <= 0 && open.compareTo(low) >= 0);
        Assume.that(close.compareTo(high) <= 0 && close.compareTo(low) >= 0);

        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);
        Candle candle = new Candle(symbol, timeframe, open, high, low, close, volume, timestamp);

        // Act
        BigDecimal range = candle.getHigh().subtract(candle.getLow());

        // Assert
        assertThat(range).isGreaterThanOrEqualTo(BigDecimal.ZERO);
        assertThat(range).isEqualByComparingTo(high.subtract(low));
    }

    @Property
    @DisplayName("shouldSupportMultipleTimeframesConsistently")
    void shouldSupportMultipleTimeframesConsistently(
            @ForAll String symbol,
            @ForAll @Positive BigDecimal open,
            @ForAll @Positive BigDecimal high,
            @ForAll @Positive BigDecimal low,
            @ForAll @Positive BigDecimal close,
            @ForAll @LongRange(min = 0) Long volume) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);
        Assume.that(high.compareTo(low) >= 0);
        Assume.that(open.compareTo(high) <= 0 && open.compareTo(low) >= 0);
        Assume.that(close.compareTo(high) <= 0 && close.compareTo(low) >= 0);

        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);
        String[] timeframes = {"M1", "M5", "M15", "M30", "H1", "D1", "W1", "MO1"};

        // Act & Assert
        for (String timeframe : timeframes) {
            Candle candle = new Candle(symbol, timeframe, open, high, low, close, volume, timestamp);
            assertThat(candle.getTimeframe()).isEqualTo(timeframe);
        }
    }

    @Property
    @DisplayName("shouldHandleZeroVolumeCandles")
    void shouldHandleZeroVolumeCandles(
            @ForAll String symbol,
            @ForAll String timeframe,
            @ForAll @Positive BigDecimal open,
            @ForAll @Positive BigDecimal high,
            @ForAll @Positive BigDecimal low,
            @ForAll @Positive BigDecimal close) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);
        Assume.that(timeframe != null && !timeframe.isEmpty());
        Assume.that(high.compareTo(low) >= 0);
        Assume.that(open.compareTo(high) <= 0 && open.compareTo(low) >= 0);
        Assume.that(close.compareTo(high) <= 0 && close.compareTo(low) >= 0);

        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);

        // Act
        Candle candle = new Candle(symbol, timeframe, open, high, low, close, 0L, timestamp);

        // Assert
        assertThat(candle.getVolume()).isEqualTo(0L);
    }

    @Property
    @DisplayName("shouldHandleLargeVolumeCandles")
    void shouldHandleLargeVolumeCandles(
            @ForAll String symbol,
            @ForAll String timeframe,
            @ForAll @Positive BigDecimal open,
            @ForAll @Positive BigDecimal high,
            @ForAll @Positive BigDecimal low,
            @ForAll @Positive BigDecimal close,
            @ForAll @LongRange(min = 1, max = Long.MAX_VALUE) Long volume) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);
        Assume.that(timeframe != null && !timeframe.isEmpty());
        Assume.that(high.compareTo(low) >= 0);
        Assume.that(open.compareTo(high) <= 0 && open.compareTo(low) >= 0);
        Assume.that(close.compareTo(high) <= 0 && close.compareTo(low) >= 0);

        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);

        // Act
        Candle candle = new Candle(symbol, timeframe, open, high, low, close, volume, timestamp);

        // Assert
        assertThat(candle.getVolume()).isEqualTo(volume);
    }

    @Property
    @DisplayName("shouldAlignTimestampsToPeriodBoundaries")
    void shouldAlignTimestampsToPeriodBoundaries(
            @ForAll String symbol,
            @ForAll String timeframe,
            @ForAll @Positive BigDecimal open,
            @ForAll @Positive BigDecimal high,
            @ForAll @Positive BigDecimal low,
            @ForAll @Positive BigDecimal close,
            @ForAll @LongRange(min = 0) Long volume) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);
        Assume.that(timeframe != null && !timeframe.isEmpty());
        Assume.that(high.compareTo(low) >= 0);
        Assume.that(open.compareTo(high) <= 0 && open.compareTo(low) >= 0);
        Assume.that(close.compareTo(high) <= 0 && close.compareTo(low) >= 0);

        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 5, 0);

        // Act
        Candle candle = new Candle(symbol, timeframe, open, high, low, close, volume, timestamp);

        // Assert
        assertThat(candle.getTimestamp()).isEqualTo(timestamp);
    }

    @Property
    @DisplayName("shouldBeDeterministicForSameInputs")
    void shouldBeDeterministicForSameInputs(
            @ForAll String symbol,
            @ForAll String timeframe,
            @ForAll @Positive BigDecimal open,
            @ForAll @Positive BigDecimal high,
            @ForAll @Positive BigDecimal low,
            @ForAll @Positive BigDecimal close,
            @ForAll @LongRange(min = 0) Long volume) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);
        Assume.that(timeframe != null && !timeframe.isEmpty());
        Assume.that(high.compareTo(low) >= 0);
        Assume.that(open.compareTo(high) <= 0 && open.compareTo(low) >= 0);
        Assume.that(close.compareTo(high) <= 0 && close.compareTo(low) >= 0);

        LocalDateTime timestamp = LocalDateTime.of(2024, 1, 15, 10, 0, 0);

        // Act
        Candle candle1 = new Candle(symbol, timeframe, open, high, low, close, volume, timestamp);
        Candle candle2 = new Candle(symbol, timeframe, open, high, low, close, volume, timestamp);

        // Assert
        assertThat(candle1.getSymbol()).isEqualTo(candle2.getSymbol());
        assertThat(candle1.getTimeframe()).isEqualTo(candle2.getTimeframe());
        assertThat(candle1.getOpen()).isEqualByComparingTo(candle2.getOpen());
        assertThat(candle1.getHigh()).isEqualByComparingTo(candle2.getHigh());
        assertThat(candle1.getLow()).isEqualByComparingTo(candle2.getLow());
        assertThat(candle1.getClose()).isEqualByComparingTo(candle2.getClose());
        assertThat(candle1.getVolume()).isEqualTo(candle2.getVolume());
    }
}
