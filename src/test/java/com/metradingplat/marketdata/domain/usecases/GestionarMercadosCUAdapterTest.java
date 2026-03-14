package com.metradingplat.marketdata.domain.usecases;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

import java.util.List;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumMercado;
import com.metradingplat.marketdata.domain.models.ActiveEquity;

@ExtendWith(MockitoExtension.class)
class GestionarMercadosCUAdapterTest {

    @Mock
    private GestionarComunicacionExternalGatewayIntPort gateway;

    private GestionarMercadosCUAdapter adapter;

    @BeforeEach
    void setUp() {
        adapter = new GestionarMercadosCUAdapter(gateway);
    }

    @Test
    void listarMercados_returnsAllEnumValues() {
        List<EnumMercado> result = adapter.listarMercados();

        assertEquals(EnumMercado.values().length, result.size());
        assertTrue(result.contains(EnumMercado.NYSE));
        assertTrue(result.contains(EnumMercado.NASDAQ));
    }

    @Test
    void obtenerSimbolosPorMercados_loadsFromGatewayOnFirstCall() {
        ActiveEquity aapl = ActiveEquity.builder().symbol("AAPL").description("Apple").listedMarket("XNAS").build();
        ActiveEquity msft = ActiveEquity.builder().symbol("MSFT").description("Microsoft").listedMarket("XNAS").build();
        ActiveEquity ibm = ActiveEquity.builder().symbol("IBM").description("IBM").listedMarket("XNYS").build();

        // Single page of results (less than PAGE_SIZE=1000)
        when(gateway.getActiveEquities(0, 1000)).thenReturn(List.of(aapl, msft, ibm));

        List<ActiveEquity> result = adapter.obtenerSimbolosPorMercados(List.of("NASDAQ"));

        assertEquals(2, result.size());
        assertTrue(result.stream().allMatch(eq -> "XNAS".equals(eq.getListedMarket())));
    }

    @Test
    void obtenerSimbolosPorMercados_returnsAllWhenNoFilterSpecified() {
        ActiveEquity aapl = ActiveEquity.builder().symbol("AAPL").listedMarket("XNAS").build();
        ActiveEquity ibm = ActiveEquity.builder().symbol("IBM").listedMarket("XNYS").build();

        when(gateway.getActiveEquities(0, 1000)).thenReturn(List.of(aapl, ibm));

        List<ActiveEquity> result = adapter.obtenerSimbolosPorMercados(List.of());

        assertEquals(2, result.size());
    }

    @Test
    void obtenerSimbolosPorMercados_usesCacheOnSecondCall() {
        ActiveEquity aapl = ActiveEquity.builder().symbol("AAPL").listedMarket("XNAS").build();

        when(gateway.getActiveEquities(0, 1000)).thenReturn(List.of(aapl));

        // First call loads from gateway
        adapter.obtenerSimbolosPorMercados(List.of());
        // Second call should use cache
        adapter.obtenerSimbolosPorMercados(List.of());

        // Gateway called only once (for page 0)
        verify(gateway, times(1)).getActiveEquities(0, 1000);
    }

    @Test
    void obtenerSimbolosPorMercados_paginatesThroughAllPages() {
        // First page: full page (1000 items)
        List<ActiveEquity> page0 = new java.util.ArrayList<>();
        for (int i = 0; i < 1000; i++) {
            page0.add(ActiveEquity.builder().symbol("SYM" + i).listedMarket("XNYS").build());
        }
        // Second page: partial page (500 items) → signals end
        List<ActiveEquity> page1 = new java.util.ArrayList<>();
        for (int i = 0; i < 500; i++) {
            page1.add(ActiveEquity.builder().symbol("SYM" + (1000 + i)).listedMarket("XNYS").build());
        }

        when(gateway.getActiveEquities(0, 1000)).thenReturn(page0);
        when(gateway.getActiveEquities(1, 1000)).thenReturn(page1);

        List<ActiveEquity> result = adapter.obtenerSimbolosPorMercados(List.of("NYSE"));

        assertEquals(1500, result.size());
        verify(gateway).getActiveEquities(0, 1000);
        verify(gateway).getActiveEquities(1, 1000);
    }
}
