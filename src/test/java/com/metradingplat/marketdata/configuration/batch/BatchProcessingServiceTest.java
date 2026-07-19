package com.metradingplat.marketdata.configuration.batch;

import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;

import static org.assertj.core.api.Assertions.assertThat;

class BatchProcessingServiceTest {

    @Test
    void shouldProcessBatchWhenSizeReached() throws InterruptedException {
        List<List<Integer>> processedBatches = new ArrayList<>();
        CountDownLatch latch = new CountDownLatch(1);
        
        BatchProcessingService<Integer> service = new BatchProcessingService<>(
                3,
                5000,
                batch -> {
                    processedBatches.add(new ArrayList<>(batch));
                    latch.countDown();
                }
        );
        
        service.add(1);
        service.add(2);
        service.add(3);
        
        boolean completed = latch.await(2, TimeUnit.SECONDS);
        assertThat(completed).isTrue();
        assertThat(processedBatches).hasSize(1);
        assertThat(processedBatches.get(0)).containsExactly(1, 2, 3);
        
        service.shutdown();
    }

    @Test
    void shouldProcessBatchOnTimeout() throws InterruptedException {
        List<List<Integer>> processedBatches = new ArrayList<>();
        CountDownLatch latch = new CountDownLatch(1);
        
        BatchProcessingService<Integer> service = new BatchProcessingService<>(
                10,
                500,
                batch -> {
                    processedBatches.add(new ArrayList<>(batch));
                    latch.countDown();
                }
        );
        
        service.add(1);
        service.add(2);
        
        boolean completed = latch.await(2, TimeUnit.SECONDS);
        assertThat(completed).isTrue();
        assertThat(processedBatches).hasSize(1);
        assertThat(processedBatches.get(0)).containsExactly(1, 2);
        
        service.shutdown();
    }

    @Test
    void shouldTrackQueueSize() {
        BatchProcessingService<Integer> service = new BatchProcessingService<>(
                5,
                5000,
                batch -> {}
        );
        
        service.add(1);
        service.add(2);
        
        assertThat(service.getQueueSize()).isEqualTo(2);
        
        service.shutdown();
    }

    @Test
    void shouldReportRunningStatus() {
        BatchProcessingService<Integer> service = new BatchProcessingService<>(
                5,
                5000,
                batch -> {}
        );
        
        assertThat(service.isRunning()).isTrue();
        
        service.shutdown();
        
        assertThat(service.isRunning()).isFalse();
    }

    @Test
    void shouldThrowExceptionWhenAddingAfterShutdown() {
        BatchProcessingService<Integer> service = new BatchProcessingService<>(
                5,
                5000,
                batch -> {}
        );
        
        service.shutdown();
        
        try {
            service.add(1);
            assertThat(true).isFalse();
        } catch (IllegalStateException e) {
            assertThat(e.getMessage()).contains("not running");
        }
    }
}
