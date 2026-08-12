# Stage 1: Build
FROM maven:3.9-eclipse-temurin-21 AS build
WORKDIR /app
COPY pom.xml .
COPY src ./src
RUN mvn clean package -DskipTests -B -Dmaven.wagon.http.retryHandler.count=3 -Dmaven.wagon.http.connectionTimeout=60000 -Dmaven.wagon.http.readTimeout=60000

# Stage 2: Production
FROM eclipse-temurin:21-jre-alpine
LABEL project="metradingplat"
LABEL service="marketdata-service"
WORKDIR /app
RUN addgroup -S spring && adduser -S spring -G spring
RUN mkdir -p /app/secedgar-cache && chown spring:spring /app/secedgar-cache
USER spring:spring
COPY --from=build /app/target/*.jar app.jar
EXPOSE 8082
# jdk.virtualThreadScheduler.parallelism por defecto es solo availableProcessors()
# (3-4 carriers en este contenedor) -- con un servicio que hace tantas
# llamadas bloqueantes de red (TastyTrade, DxLink, SEC EDGAR, Kafka), muy
# pocos hilos virtuales concurrentes bastan para dejar sin carriers libres a
# TODO el servicio, incluido /actuator/health (confirmado en vivo con un
# thread dump real). Mas margen aqui es la red de seguridad; el arreglo de
# fondo es no fijar el carrier con Thread.sleep/blocking dentro de un
# synchronized (ver CandleChannelAllocator).
ENTRYPOINT ["java", "-XX:+UseContainerSupport", "-Xms256m", "-Xmx1536m", "-XX:+UseG1GC", "-XX:ParallelGCThreads=1", "-XX:ConcGCThreads=1", "-Djdk.virtualThreadScheduler.parallelism=32", "-jar", "app.jar"]
