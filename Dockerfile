# Stage 1: Build
FROM golang:1.25 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -o /app/marketdata-service ./cmd/api

# Stage 2: Production
FROM alpine:3.20
LABEL project="metradingplat"
LABEL service="marketdata-service"
RUN apk add --no-cache ca-certificates && addgroup -S app && adduser -S app -G app
WORKDIR /app
USER app
COPY --from=build /app/marketdata-service .
EXPOSE 8082
ENTRYPOINT ["./marketdata-service"]
