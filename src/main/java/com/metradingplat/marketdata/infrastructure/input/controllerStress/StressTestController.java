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

    // DIAGNOSTICO TEMPORAL -- se elimina despues de la prueba.
    @PostMapping("/candle-live-probe/start")
    public Map<String, Object> startCandleLiveProbe(@RequestParam(defaultValue = "5") int connections) {
        return stressTestService.startCandleLiveProbe(connections);
    }

    @GetMapping("/candle-live-probe/status")
    public Map<String, Object> getCandleLiveProbeStatus() {
        return stressTestService.getCandleLiveProbeStatus();
    }

    @PostMapping("/candle-live-probe/stop")
    public Map<String, Object> stopCandleLiveProbe() {
        return stressTestService.stopCandleLiveProbe();
    }
}
