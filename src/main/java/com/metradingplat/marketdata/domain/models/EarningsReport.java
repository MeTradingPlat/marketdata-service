package com.metradingplat.marketdata.domain.models;

import java.time.LocalDate;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@AllArgsConstructor
@NoArgsConstructor
public class EarningsReport {
    private String symbol;
    private LocalDate occurredDate;
    private Double eps;
    private Long daysUntilEarnings;
}
