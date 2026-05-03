package com.metradingplat.marketdata.infrastructure.input.controllerGestionarFundamentals.DTOAnswer;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@AllArgsConstructor
@NoArgsConstructor
public class FundamentalDataDTORespuesta {
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
    private Double eps;
    private Double dividendAmount;
    private String dividendFrequency;
    private Double open;
    private Double high;
    private Double low;
    private Double prevClose;
    private String tradingStatus;
    private String statusReason;
    private Long haltStartTime;
    private Long haltEndTime;
    private Double beta;
    
    // Market Metrics
    private Double impliedVolatilityIndex;
    private Double impliedVolatilityRank;
    private Double impliedVolatilityPercentile;
    private Double liquidity;
    private Integer liquidityRating;
}
