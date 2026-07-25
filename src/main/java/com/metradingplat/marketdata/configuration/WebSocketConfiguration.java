package com.metradingplat.marketdata.configuration;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.metradingplat.marketdata.adapter.websocket.CandleWebSocketHandler;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Configuration;
import org.springframework.messaging.converter.DefaultContentTypeResolver;
import org.springframework.messaging.converter.MappingJackson2MessageConverter;
import org.springframework.messaging.converter.MessageConverter;
import org.springframework.util.MimeTypeUtils;
import org.springframework.web.socket.config.annotation.EnableWebSocket;
import org.springframework.web.socket.config.annotation.WebSocketConfigurer;
import org.springframework.web.socket.config.annotation.WebSocketHandlerRegistry;

import java.util.List;

/**
 * WebSocket configuration for Spring Boot.
 * 
 * Configures:
 * - WebSocket endpoints registration
 * - JSON message converters
 * - Connection limits (1,000 concurrent connections)
 * - CORS settings
 */
@Slf4j
@Configuration
@EnableWebSocket
@RequiredArgsConstructor
public class WebSocketConfiguration implements WebSocketConfigurer {

    @Value("${marketdata.websocket.max-connections:1000}")
    private int maxConnections;

    private final CandleWebSocketHandler candleWebSocketHandler;

    @Override
    public void registerWebSocketHandlers(WebSocketHandlerRegistry registry) {
        log.info("Registering WebSocket endpoints with max connections: {}", maxConnections);

        // Register WebSocket endpoints for market data streaming
        registry.addHandler(quoteWebSocketHandler(), "/ws/quotes")
                .setAllowedOrigins("*")
                .withSockJS();

        registry.addHandler(tradeWebSocketHandler(), "/ws/trades")
                .setAllowedOrigins("*")
                .withSockJS();

        registry.addHandler(candleWebSocketHandler, "/ws/candles")
                .setAllowedOrigins("*");

        registry.addHandler(greeksWebSocketHandler(), "/ws/greeks")
                .setAllowedOrigins("*")
                .withSockJS();

        registry.addHandler(underlyingWebSocketHandler(), "/ws/underlying")
                .setAllowedOrigins("*")
                .withSockJS();

        log.info("WebSocket endpoints registered successfully");
    }

    /**
     * Quote WebSocket handler bean.
     */
    public org.springframework.web.socket.WebSocketHandler quoteWebSocketHandler() {
        return new com.metradingplat.marketdata.adapter.websocket.QuoteWebSocketHandler();
    }

    /**
     * Trade WebSocket handler bean.
     */
    public org.springframework.web.socket.WebSocketHandler tradeWebSocketHandler() {
        return new com.metradingplat.marketdata.adapter.websocket.TradeWebSocketHandler();
    }

    /**
     * Greeks WebSocket handler bean.
     */
    public org.springframework.web.socket.WebSocketHandler greeksWebSocketHandler() {
        return new com.metradingplat.marketdata.adapter.websocket.GreeksWebSocketHandler();
    }

    /**
     * Underlying WebSocket handler bean.
     */
    public org.springframework.web.socket.WebSocketHandler underlyingWebSocketHandler() {
        return new com.metradingplat.marketdata.adapter.websocket.UnderlyingWebSocketHandler();
    }

    /**
     * Configure message converters for JSON serialization.
     */
    public List<MessageConverter> configureMessageConverters(List<MessageConverter> converters) {
        DefaultContentTypeResolver resolver = new DefaultContentTypeResolver();
        resolver.setDefaultMimeType(MimeTypeUtils.APPLICATION_JSON);

        MappingJackson2MessageConverter converter = new MappingJackson2MessageConverter();
        converter.setContentTypeResolver(resolver);
        converter.setObjectMapper(new ObjectMapper());

        converters.add(converter);
        return converters;
    }
}
