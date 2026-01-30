package com.metradingplat.marketdata.application.input;

import com.metradingplat.marketdata.domain.models.EarningsReport;

public interface GestionarEarningsCUIntPort {

    EarningsReport obtenerProximoEarnings(String symbol);
}
