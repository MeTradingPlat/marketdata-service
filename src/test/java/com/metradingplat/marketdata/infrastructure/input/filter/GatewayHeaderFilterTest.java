package com.metradingplat.marketdata.infrastructure.input.filter;

import static org.junit.jupiter.api.Assertions.*;

import org.junit.jupiter.api.Test;
import org.springframework.mock.web.MockHttpServletRequest;

class GatewayHeaderFilterTest {

    private final GatewayHeaderFilter filter = new GatewayHeaderFilter();

    @Test
    void shouldNotFilter_actuatorPath() {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setRequestURI("/marketdata/actuator/health");

        assertTrue(filter.shouldNotFilter(request));
    }

    @Test
    void shouldNotFilter_healthPath() {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setRequestURI("/marketdata/health/dxlink/status");

        assertTrue(filter.shouldNotFilter(request));
    }

    @Test
    void shouldFilter_regularApiPath() {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setRequestURI("/marketdata/historical/AAPL");

        assertFalse(filter.shouldNotFilter(request));
    }

    @Test
    void doFilterInternal_allowsRequestWithGatewayHeader() throws Exception {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setRequestURI("/marketdata/quote/AAPL");
        request.addHeader("X-Gateway-Passed", "true");

        org.springframework.mock.web.MockHttpServletResponse response = new org.springframework.mock.web.MockHttpServletResponse();
        org.springframework.mock.web.MockFilterChain chain = new org.springframework.mock.web.MockFilterChain();

        filter.doFilterInternal(request, response, chain);

        assertEquals(200, response.getStatus());
        assertNotNull(chain.getRequest()); // chain was invoked
    }

    @Test
    void doFilterInternal_blocksRequestWithoutGatewayHeader() throws Exception {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.setRequestURI("/marketdata/quote/AAPL");

        org.springframework.mock.web.MockHttpServletResponse response = new org.springframework.mock.web.MockHttpServletResponse();
        org.springframework.mock.web.MockFilterChain chain = new org.springframework.mock.web.MockFilterChain();

        filter.doFilterInternal(request, response, chain);

        assertEquals(403, response.getStatus());
        assertNull(chain.getRequest()); // chain was NOT invoked
    }
}
