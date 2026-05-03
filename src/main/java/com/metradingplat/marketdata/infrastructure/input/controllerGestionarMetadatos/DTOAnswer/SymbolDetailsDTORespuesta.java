package com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.DTOAnswer;

import com.metradingplat.marketdata.domain.models.FundamentalData;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class SymbolDetailsDTORespuesta {
    private ActiveEquityDTORespuesta activeEquity;
    private FundamentalData fundamentalData;
}
