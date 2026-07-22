package com.metradingplat.marketdata.domain.models;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.time.Instant;
import java.time.LocalDate;

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
    private Double open;
    private Double high;
    private Double low;
    private Double prevClose;
    private Long openInterest;
    private Integer daysUntilEarnings;
    private Long preMarketVolume;
    private Long postMarketVolume;
    private LocalDate nextEarningsDate;
    private LocalDate occurredDate;
    private Double eps;
    private Double dividendAmount;
    private String dividendFrequency;
    private String tradingStatus;
    private String statusReason;
    private Long haltStartTime;
    private Long haltEndTime;
    private Double beta;
    private Instant lastEarningsUpdated;
    private Instant lastShortInterestUpdated;
    private Instant lastUpdated;
    
    // Market Metrics Fields
    private Double impliedVolatilityIndex;
    private Double impliedVolatilityRank;
    private Double impliedVolatilityPercentile;
    private Double liquidity;
    private Integer liquidityRating;
}
