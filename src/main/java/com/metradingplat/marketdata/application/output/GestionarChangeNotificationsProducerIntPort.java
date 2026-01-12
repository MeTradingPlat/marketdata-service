package com.metradingplat.marketdata.application.output;

import com.metradingplat.marketdata.infrastructure.output.kafka.DTO.MarketDataStreamDTO;
import com.metradingplat.marketdata.infrastructure.output.kafka.DTO.OrderUpdateDTO;

public interface GestionarChangeNotificationsProducerIntPort {
    void publishOrderUpdate(OrderUpdateDTO update);

    void publishMarketData(MarketDataStreamDTO data);
}
