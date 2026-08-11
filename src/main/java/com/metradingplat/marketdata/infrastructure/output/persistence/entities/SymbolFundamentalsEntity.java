package com.metradingplat.marketdata.infrastructure.output.persistence.entities;

import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;
import java.time.LocalDate;
import java.time.Instant;

@Entity
@Table(name = "symbol_fundamentals")
@Data
@Builder
@AllArgsConstructor
@NoArgsConstructor
public class SymbolFundamentalsEntity {
    
    @Id
    private String symbol;
    
    private Double shortInterest;
    private Double shortRatio;
    
    private LocalDate nextEarningsDate;
    private LocalDate occurredDate;
    private Double eps;
    
    private Double marketCap;
    private Long sharesOutstanding;
    private Long floatShares;
    private Long dayVolume;
    private Integer daysUntilEarnings;
    private Long preMarketVolume;
    private Long postMarketVolume;
    private Double preMarketClose;
    private Double postMarketClose;
    
    private Double impliedVolatilityIndex;
    private Double impliedVolatilityRank;
    private Double impliedVolatilityPercentile;
    private Double liquidity;
    private Integer liquidityRating;
    
    private Instant lastEarningsUpdated;
    private Instant lastShortInterestUpdated;
    private Instant lastUpdated;
}
