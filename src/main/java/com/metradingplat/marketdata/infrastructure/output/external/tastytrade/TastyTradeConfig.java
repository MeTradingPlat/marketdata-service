package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.web.client.RestClient;

import lombok.Data;

@Configuration
@ConfigurationProperties(prefix = "tastytrade")
@Data
public class TastyTradeConfig {

    private String clientId;
    private String clientSecret;
    private String refreshToken;
    private String accountNumber;
    private String apiBaseUrl = "https://api.tastyworks.com";
    private String dxlinkUrl = "wss://tasty.dxfeed.com/realtime";
    private String accountStreamerUrl = "wss://streamer.tastyworks.com";
    
    private DxlinkConfig dxlink = new DxlinkConfig();
    private TokenRefreshConfig tokenRefresh = new TokenRefreshConfig();
    private CandleBurstConfig candleBurst = new CandleBurstConfig();
    private ConnectionPoolConfig connectionPool = new ConnectionPoolConfig();

    @Data
    public static class DxlinkConfig {
        private int keepaliveInterval = 30000;
        private int connectionTimeout = 10000;
        private String acceptDataFormat = "COMPACT";
    }

    @Data
    public static class TokenRefreshConfig {
        private boolean enabled = true;
        private int fixedRateHours = 23;
    }

    @Data
    public static class CandleBurstConfig {
        private int thresholdSymbols = 1600;
        // Techo GLOBAL de conexiones efimeras de velas abiertas a la vez en
        // todo el servicio (no por peticion -- una sola rafaga puede abrir
        // tantas como haga falta, este semaforo solo evita que varias
        // rafagas simultaneas de distintos escaneres sumen demasiadas
        // conexiones de golpe contra TastyTrade). 40 da margen holgado sobre
        // los ~17 que necesito una rafaga del universo completo, mas las 5
        // permanentes del pool de fundamentales.
        private int maxConcurrentConnections = 40;
    }

    @Data
    public static class ConnectionPoolConfig {
        private int connectionCount = 5;
        private int symbolsPerConnection = 2600;
    }

    @Bean
    public RestClient tastyTradeRestClient() {
        return RestClient.builder()
                .baseUrl(apiBaseUrl)
                .defaultHeader("Content-Type", "application/json")
                .defaultHeader("Accept", "application/json")
                .defaultHeader("User-Agent", "QuantMaestro/2.0.0")
                .build();
    }
}
