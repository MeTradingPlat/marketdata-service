package com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.DTOPetition;

import java.util.List;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class BatchCandlesDTOPeticion {
    private List<String> symbols;
    private EnumTimeframe timeframe;
    private Integer bars;
}
