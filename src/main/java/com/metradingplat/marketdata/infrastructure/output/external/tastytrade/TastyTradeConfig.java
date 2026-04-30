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
