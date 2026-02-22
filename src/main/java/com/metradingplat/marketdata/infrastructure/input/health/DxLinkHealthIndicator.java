package com.metradingplat.marketdata.infrastructure.input.health;

import com.metradingplat.marketdata.infrastructure.output.external.tastytrade.DxLinkClient;
import lombok.RequiredArgsConstructor;
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.HealthIndicator;
import org.springframework.stereotype.Component;

@Component
@RequiredArgsConstructor
public class DxLinkHealthIndicator implements HealthIndicator {

    private final DxLinkClient dxLinkClient;

    @Override
    public Health health() {
        if (dxLinkClient.isConnected()) {
            return Health.up()
                    .withDetail("DxLink Connection", "Connected")
                    .withDetail("Service", "MarketData")
                    .build();
        } else {
            return Health.down()
                    .withDetail("DxLink Connection", "Disconnected")
                    .withDetail("Error", "WebSocket to dxFeed is offline")
                    .build();
        }
    }
}
