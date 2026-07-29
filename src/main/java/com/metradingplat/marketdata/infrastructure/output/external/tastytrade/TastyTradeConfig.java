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
    private CandlePoolConfig candlePool = new CandlePoolConfig();
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
    public static class CandlePoolConfig {
        // Techo GLOBAL de conexiones persistentes de velas que el pool puede
        // tener abiertas a la vez en todo el servicio. A diferencia de la
        // vieja rafaga efimera, estas conexiones se quedan abiertas mientras
        // algun simbolo siga activo -- el limite evita que el pool crezca sin
        // control si muchos escaneres terminan cubriendo casi todo el
        // universo en varias temporalidades a la vez. 40 da margen holgado
        // sobre los ~17 que mide una cobertura de universo completo en una
        // sola temporalidad, mas las 5 permanentes del pool de fundamentales.
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
