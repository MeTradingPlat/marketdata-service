package com.metradingplat.marketdata.application.input;

import com.metradingplat.marketdata.domain.models.Quote;

public interface GestionarQuoteCUIntPort {

    Quote obtenerQuote(String symbol);
}
