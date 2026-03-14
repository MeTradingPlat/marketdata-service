package com.metradingplat.marketdata;

import org.junit.jupiter.api.Test;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.test.context.ActiveProfiles;
import org.springframework.test.context.bean.override.mockito.MockitoBean;

@SpringBootTest
@ActiveProfiles("test")
class MarketdataServiceApplicationTests {

	@MockitoBean
	private KafkaTemplate<String, Object> kafkaTemplate;

	@Test
	void contextLoads() {
	}

}
