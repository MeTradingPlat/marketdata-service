package com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.DTOAnswer;

import java.time.Instant;
import java.util.Map;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class BatchSingleCandleDTORespuesta {
    private Map<String, CandleDTORespuesta> candlePorSimbolo;
    private Instant serverTimestamp;
}
