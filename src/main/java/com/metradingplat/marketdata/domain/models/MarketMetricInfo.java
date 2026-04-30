package com.metradingplat.marketdata.domain.models;

import lombok.Builder;
import lombok.Data;

@Data
@Builder
public class MarketMetricInfo {
    private String symbol;
    private Double impliedVolatilityIndex;
    private Double impliedVolatilityRank;
    private Double impliedVolatilityPercentile;
    private Double liquidity;
    private Integer liquidityRating;
}
