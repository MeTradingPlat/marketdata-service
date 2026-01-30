package com.metradingplat.marketdata.domain.usecases;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.Set;
import java.util.stream.Collectors;

import com.metradingplat.marketdata.application.input.GestionarMercadosCUIntPort;
import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumMercado;
import com.metradingplat.marketdata.domain.models.ActiveEquity;

import lombok.RequiredArgsConstructor;

@RequiredArgsConstructor
public class GestionarMercadosCUAdapter implements GestionarMercadosCUIntPort {

    private final GestionarComunicacionExternalGatewayIntPort objExternalGateway;

    @Override
    public List<EnumMercado> listarMercados() {
        return Arrays.asList(EnumMercado.values());
    }

    @Override
    public List<ActiveEquity> obtenerSimbolosPorMercados(List<String> markets) {
        Set<String> marketFilter = markets != null && !markets.isEmpty()
                ? markets.stream().map(String::toUpperCase).collect(Collectors.toSet())
                : Set.of();

        List<ActiveEquity> allEquities = new ArrayList<>();
        int page = 0;
        int perPage = 1000;

        while (true) {
            List<ActiveEquity> batch = this.objExternalGateway.getActiveEquities(page, perPage);
            if (batch.isEmpty()) break;

            if (marketFilter.isEmpty()) {
                allEquities.addAll(batch);
            } else {
                for (ActiveEquity eq : batch) {
                    if (eq.getListedMarket() != null && marketFilter.contains(eq.getListedMarket().toUpperCase())) {
                        allEquities.add(eq);
                    }
                }
            }
            if (batch.size() < perPage) break;
            page++;
        }

        return allEquities;
    }
}
