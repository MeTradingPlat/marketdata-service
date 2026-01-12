package com.metradingplat.marketdata.infrastructure.output.kafka.producer;

import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.stereotype.Service;

import com.metradingplat.marketdata.application.output.GestionarChangeNotificationsProducerIntPort;
import com.metradingplat.marketdata.infrastructure.output.kafka.DTO.MarketDataStreamDTO;
import com.metradingplat.marketdata.infrastructure.output.kafka.DTO.OrderUpdateDTO;

import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

@Service
@Slf4j
@RequiredArgsConstructor
public class KafkaProducerAdapter implements GestionarChangeNotificationsProducerIntPort {

    private final KafkaTemplate<String, Object> kafkaTemplate;

    private static final String ORDERS_UPDATES_TOPIC = "orders.updates";
    private static final String MARKETDATA_STREAM_TOPIC = "marketdata.stream";

    @Override
    public void publishOrderUpdate(OrderUpdateDTO update) {
        log.info("Publishing order update for symbol: {}", update.getSymbol());
        kafkaTemplate.send(ORDERS_UPDATES_TOPIC, update.getOrderId(), update);
    }

    @Override
    public void publishMarketData(MarketDataStreamDTO data) {
        // Log is too verbose for stream, maybe trace
        kafkaTemplate.send(MARKETDATA_STREAM_TOPIC, data.getSymbol(), data);
    }
}
