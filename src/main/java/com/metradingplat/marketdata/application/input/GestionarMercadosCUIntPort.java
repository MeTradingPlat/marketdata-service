package com.metradingplat.marketdata.application.input;

import java.util.List;

import com.metradingplat.marketdata.domain.enums.EnumMercado;
import com.metradingplat.marketdata.domain.models.ActiveEquity;

public interface GestionarMercadosCUIntPort {

    List<EnumMercado> listarMercados();

    List<ActiveEquity> obtenerSimbolosPorMercados(List<String> markets);
}
