package com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.DTOAnswer;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ActiveEquityDTORespuesta {
    private String symbol;
    private String description;
    private String listedMarket;
}
