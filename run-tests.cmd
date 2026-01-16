@echo off
REM Script para ejecutar tests con Java 21

echo ========================================
echo MarketData Service - Run Tests
echo ========================================
echo.

REM Configurar JAVA_HOME para Java 21
set JAVA_HOME=C:\Program Files\Eclipse Adoptium\jdk-21.0.9.10-hotspot
set PATH=%JAVA_HOME%\bin;%PATH%

echo Using Java: %JAVA_HOME%
echo.

REM Verificar variables de entorno (necesario para algunos tests)
if "%TT_CLIENT_ID%"=="" (
    echo WARNING: TT_CLIENT_ID not set
    echo Some integration tests may be skipped.
    echo.
)

echo Running tests...
echo.

mvnw.cmd test

echo.
echo ========================================
if %ERRORLEVEL% EQU 0 (
    echo All tests passed!
) else (
    echo Some tests failed!
)
echo ========================================

pause
