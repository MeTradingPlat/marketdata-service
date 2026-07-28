package com.metradingplat.marketdata.infrastructure.input.controllerStress;

import com.metradingplat.marketdata.infrastructure.output.external.tastytrade.StressTestService;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.*;

import java.util.List;
import java.util.Map;
import java.util.concurrent.CompletableFuture;

@RestController
@RequestMapping("/marketdata/stress")
@RequiredArgsConstructor
public class StressTestController {

    private final StressTestService stressTestService;

    @GetMapping("/stats")
    public Map<String, Object> getStats() {
        return stressTestService.getSystemStats();
    }

    @PostMapping("/subscribe-massive")
    public Map<String, Object> subscribeMassive(@RequestBody List<String> symbols) {
        return stressTestService.subscribeMassive(symbols);
    }

    @GetMapping("/validate-orders-burst")
    public CompletableFuture<Map<String, Object>> validateOrdersBurst(@RequestParam(defaultValue = "10") int count) {
        return stressTestService.validateOrdersBurst(count);
    }

    @PostMapping("/fundamentals-channel-capacity")
    public Map<String, Object> testFundamentalsChannelCapacity(@RequestBody List<String> symbols,
            @RequestParam(defaultValue = "60") int waitSeconds) {
        return stressTestService.testFundamentalsChannelCapacity(symbols, waitSeconds);
    }
}
