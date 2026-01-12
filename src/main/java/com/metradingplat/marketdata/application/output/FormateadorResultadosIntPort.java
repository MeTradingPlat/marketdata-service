package com.metradingplat.marketdata.application.output;

public interface FormateadorResultadosIntPort {
    public void errorEntidadYaExiste(String llaveMensaje, Object... args);

    public void errorEntidadNoExiste(String llaveMensaje, Object... args);

    public void errorEstadoDenegado(String llaveMensaje, Object... args);

    public void errorReglaNegocioViolada(String llaveMensaje, Object... args);
}