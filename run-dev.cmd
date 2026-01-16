@echo off
REM Script para ejecutar el servicio en modo desarrollo
REM Asegura que se use Java 21

echo ========================================
echo MarketData Service - Development Mode
echo ========================================
echo.

REM Configurar JAVA_HOME para Java 21
set JAVA_HOME=C:\Program Files\Eclipse Adoptium\jdk-21.0.9.10-hotspot
set PATH=%JAVA_HOME%\bin;%PATH%

echo Using Java: %JAVA_HOME%
echo.

REM Verificar variables de entorno requeridas
if "%TT_CLIENT_ID%"=="" (
    echo WARNING: TT_CLIENT_ID not set
    echo.
    echo Please set environment variables:
    echo   set TT_CLIENT_ID=your_client_id
    echo   set TT_CLIENT_SECRET=your_client_secret
    echo   set TT_REFRESH_TOKEN=your_refresh_token
    echo   set TASTYTRADE_ACCOUNT_NUMBER=your_account_number
    echo.
)

echo Starting service with profile: dev
echo.

REM Ejecutar con Maven wrapper
mvnw.cmd spring-boot:run

pause
