package com.metradingplat.marketdata.domain.models;

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
public class OptionChain {
    private String symbol;
    private Double underlyingPrice;
    
    // Agrupados por fecha de expiración
    private Map<String, List<OptionContract>> expirations;
}
