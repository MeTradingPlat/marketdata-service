@echo off
REM Script para compilar el proyecto con Java 21

echo ========================================
echo MarketData Service - Compile
echo ========================================
echo.

REM Configurar JAVA_HOME para Java 21
set JAVA_HOME=C:\Program Files\Eclipse Adoptium\jdk-21.0.9.10-hotspot
set PATH=%JAVA_HOME%\bin;%PATH%

echo Using Java: %JAVA_HOME%
echo.

echo Compiling project...
echo.

mvnw.cmd clean compile -DskipTests

echo.
echo ========================================
if %ERRORLEVEL% EQU 0 (
    echo Compilation successful!
) else (
    echo Compilation failed!
)
echo ========================================

pause
