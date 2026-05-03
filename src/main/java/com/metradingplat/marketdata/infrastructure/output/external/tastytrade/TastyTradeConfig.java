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
