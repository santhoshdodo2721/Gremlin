FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -o /resiletops ./cmd/gremlin

FROM alpine:3.19
WORKDIR /app
COPY --from=build /resiletops /app/resiletops
COPY monitoring/exporter-config.json /app/config.json
EXPOSE 9090
ENTRYPOINT ["/app/resiletops", "serve-metrics"]
