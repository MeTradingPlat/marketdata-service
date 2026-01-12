package com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.DTOAnswer;

import java.time.Instant;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class CandleDTORespuesta {
    private String symbol;
    private Instant timestamp;
    private Double open;
    private Double high;
    private Double low;
    private Double close;
    private Double volume;
}
