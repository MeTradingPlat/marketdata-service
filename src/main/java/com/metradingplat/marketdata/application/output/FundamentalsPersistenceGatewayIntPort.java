package com.metradingplat.marketdata.application.output;

import com.metradingplat.marketdata.domain.models.FundamentalData;
import java.util.List;
import java.util.Optional;

public interface FundamentalsPersistenceGatewayIntPort {
    Optional<FundamentalData> findBySymbol(String symbol);
    FundamentalData save(FundamentalData fundamentalData);

    /** Solo simbolos con algo de pre/post-market guardado -- usado para
     * rehidratar la cache en memoria al arrancar. */
    List<FundamentalData> findAllWithExtendedHoursData();

    /** Reset diario: limpia pre/post-market volume y close para TODOS los
     * simbolos, igual que resetDailyExtendedHoursVolume() ya hace en memoria. */
    void clearExtendedHoursData();
}
