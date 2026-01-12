package com.metradingplat.marketdata.infrastructure.input.kafkaGestionarRealTime.DTOPetition;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class RealTimeRequestDTO {
    private String action;
    private String symbol;
}
