package com.metradingplat.marketdata.application.input;

import java.util.List;
import java.util.Map;

import com.metradingplat.marketdata.domain.models.FundamentalData;

public interface GestionarFundamentalsCUIntPort {
    FundamentalData obtenerFundamentals(String symbol);
    Map<String, FundamentalData> obtenerFundamentalsBatch(List<String> symbols);
}
