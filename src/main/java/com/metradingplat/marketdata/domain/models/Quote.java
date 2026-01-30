package com.metradingplat.marketdata.domain.models;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@AllArgsConstructor
@NoArgsConstructor
public class Quote {
    private String symbol;
    private Double bid;
    private Double ask;
    private Double last;
    private Double open;
    private Double high;
    private Double low;
    private Double close;
    private Double prevClose;
    private Double volume;
    private Boolean tradingHalted;
    private String tradingHaltedReason;
    private Double beta;
}
