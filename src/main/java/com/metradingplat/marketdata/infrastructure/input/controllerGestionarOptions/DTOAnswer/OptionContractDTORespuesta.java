package com.metradingplat.marketdata.infrastructure.input.controllerGestionarOptions.DTOAnswer;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.time.LocalDate;

@Data
@Builder
@AllArgsConstructor
@NoArgsConstructor
public class OptionContractDTORespuesta {
    private String symbol;
    private String rootSymbol;
    private Double strikePrice;
    private LocalDate expirationDate;
    private String optionType;
    
    // Griegas
    private Double delta;
    private Double gamma;
    private Double theta;
    private Double vega;
    private Double rho;
    private Double impliedVolatility;
    private Double theoreticalPrice;
    
    // Mercado
    private Double bid;
    private Double ask;
    private Double last;
    private Long volume;
    private Long openInterest;
}
