package com.metradingplat.marketdata.domain.models;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.time.LocalDate;

@Data
@Builder
@AllArgsConstructor
@NoArgsConstructor
public class OptionContract {
    private String symbol;          // OSI Symbol (ej: AAPL  240517C00190000)
    private String rootSymbol;      // ej: AAPL
    private Double strikePrice;
    private LocalDate expirationDate;
    private String optionType;      // CALL / PUT
    
    // Datos en tiempo real (Griegas)
    private Double delta;
    private Double gamma;
    private Double theta;
    private Double vega;
    private Double rho;
    private Double impliedVolatility;
    private Double theoreticalPrice;
    
    // Datos de Mercado
    private Double bid;
    private Double ask;
    private Double last;
    private Long volume;
    private Long openInterest;
}
