package com.metradingplat.marketdata.infrastructure.input.controllerGestionarQuote.DTOAnswer;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class QuoteDTORespuesta {
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
