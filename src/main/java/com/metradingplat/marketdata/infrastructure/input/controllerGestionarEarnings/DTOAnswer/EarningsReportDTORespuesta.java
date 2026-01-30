package com.metradingplat.marketdata.infrastructure.input.controllerGestionarEarnings.DTOAnswer;

import java.time.LocalDate;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class EarningsReportDTORespuesta {
    private String symbol;
    private LocalDate occurredDate;
    private Double eps;
    private Long daysUntilEarnings;
}
