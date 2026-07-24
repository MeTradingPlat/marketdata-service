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
ENTRYPOINT ["java", "-XX:+UseContainerSupport", "-Xms64m", "-Xmx256m", "-XX:+UseG1GC", "-XX:ParallelGCThreads=1", "-XX:ConcGCThreads=1", "-jar", "app.jar"]
