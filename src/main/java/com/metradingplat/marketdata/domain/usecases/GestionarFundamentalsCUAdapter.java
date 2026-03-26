package com.metradingplat.marketdata.domain.usecases;

import java.util.List;
import java.util.Map;

import com.metradingplat.marketdata.application.input.GestionarFundamentalsCUIntPort;
import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.models.FundamentalData;

import lombok.RequiredArgsConstructor;

@RequiredArgsConstructor
public class GestionarFundamentalsCUAdapter implements GestionarFundamentalsCUIntPort {

    private final GestionarComunicacionExternalGatewayIntPort objExternalGateway;

    @Override
    public FundamentalData obtenerFundamentals(String symbol) {
        Map<String, FundamentalData> result = this.objExternalGateway.getFundamentalsBatch(List.of(symbol));
        return result.getOrDefault(symbol, FundamentalData.builder().symbol(symbol).build());
    }

    @Override
    public Map<String, FundamentalData> obtenerFundamentalsBatch(List<String> symbols) {
        return this.objExternalGateway.getFundamentalsBatch(symbols);
    }
}
