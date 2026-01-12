package com.metradingplat.marketdata.domain.models;

import java.time.Instant;
import java.time.LocalDateTime;
import java.time.ZoneId;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class Candle {
    private String symbol;
    private EnumTimeframe timeframe;
    private Instant timestamp;
    private Double open;
    private Double high;
    private Double low;
    private Double close;
    private Double volume;

    public LocalDateTime getLocalDateTime() {
        return LocalDateTime.ofInstant(timestamp, ZoneId.systemDefault());
    }
}