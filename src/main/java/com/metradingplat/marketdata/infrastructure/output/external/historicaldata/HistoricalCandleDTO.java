package com.metradingplat.marketdata.infrastructure.output.external.historicaldata;

import java.time.Instant;

import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@NoArgsConstructor
public class HistoricalCandleDTO {
    private String symbol;
    private Instant timestamp;
    private Double open;
    private Double high;
    private Double low;
    private Double close;
    private Double volume;
    private Double vwap;
    private String source;
}
