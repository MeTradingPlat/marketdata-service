package com.metradingplat.marketdata.application.output;

import com.metradingplat.marketdata.domain.models.FundamentalData;
import java.util.Optional;

public interface FundamentalsPersistenceGatewayIntPort {
    Optional<FundamentalData> findBySymbol(String symbol);
    FundamentalData save(FundamentalData fundamentalData);
}
