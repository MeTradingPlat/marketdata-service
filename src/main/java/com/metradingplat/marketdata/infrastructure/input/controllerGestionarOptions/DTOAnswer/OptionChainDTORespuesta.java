package com.metradingplat.marketdata.infrastructure.input.controllerGestionarOptions.DTOAnswer;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.List;
import java.util.Map;

@Data
@Builder
@AllArgsConstructor
@NoArgsConstructor
public class OptionChainDTORespuesta {
    private String symbol;
    private Double underlyingPrice;
    private Map<String, List<OptionContractDTORespuesta>> expirations;
}
