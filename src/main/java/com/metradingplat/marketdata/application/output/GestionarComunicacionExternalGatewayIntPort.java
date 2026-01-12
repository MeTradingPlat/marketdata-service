package com.metradingplat.marketdata.application.output;

import java.time.OffsetDateTime;
import java.util.List;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.domain.models.OrderRequest;

public interface GestionarComunicacionExternalGatewayIntPort {
    void sendOrder(OrderRequest request);

    void subscribe(String symbol);

    void unsubscribe(String symbol);

    List<Candle> getCandles(String symbol, EnumTimeframe timeframe, OffsetDateTime from, OffsetDateTime to);
}
