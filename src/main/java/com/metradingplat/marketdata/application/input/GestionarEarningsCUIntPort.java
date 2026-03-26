package com.metradingplat.marketdata.application.input;

import java.util.List;
import java.util.Map;

import com.metradingplat.marketdata.domain.models.EarningsReport;

public interface GestionarEarningsCUIntPort {

    EarningsReport obtenerProximoEarnings(String symbol);
    Map<String, EarningsReport> obtenerEarningsBatch(List<String> symbols);
}
