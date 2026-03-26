package com.metradingplat.marketdata.domain.models;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@AllArgsConstructor
@NoArgsConstructor
public class FundamentalData {
    private String symbol;
    private Double marketCap;
    private Long sharesOutstanding;
    private Long floatShares;
    private Double shortInterest;
    private Double shortRatio;
    private Long dayVolume;
    private Integer daysUntilEarnings;
    private Long preMarketVolume;
    private Long postMarketVolume;
}
