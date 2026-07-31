package com.metradingplat.marketdata.application.input;

import java.util.List;

import com.metradingplat.marketdata.domain.enums.EnumMercado;
import com.metradingplat.marketdata.domain.models.ActiveEquity;
import com.metradingplat.marketdata.domain.models.PagedActiveEquities;

public interface GestionarMercadosCUIntPort {

    List<EnumMercado> listarMercados();

    List<String> obtenerMercadosDisponibles();

    List<ActiveEquity> obtenerSimbolosPorMercados(List<String> markets);

    PagedActiveEquities buscarSimbolos(String query, List<String> markets, int page, int size);
}
