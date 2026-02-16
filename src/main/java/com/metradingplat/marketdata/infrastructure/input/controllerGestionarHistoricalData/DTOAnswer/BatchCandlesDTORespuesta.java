package com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.DTOAnswer;

import java.time.Instant;
import java.util.List;
import java.util.Map;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class BatchCandlesDTORespuesta {
    private Map<String, List<CandleDTORespuesta>> candlesPorSimbolo;
    private Instant serverTimestamp;
}
