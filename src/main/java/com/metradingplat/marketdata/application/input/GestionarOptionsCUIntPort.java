package com.metradingplat.marketdata.application.input;

import com.metradingplat.marketdata.domain.models.OptionChain;

public interface GestionarOptionsCUIntPort {
    OptionChain obtenerOptionChain(String symbol);
}
