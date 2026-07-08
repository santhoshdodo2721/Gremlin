FROM golang:1.22-alpine AS build
WORKDIR /src
COPY main.go .
RUN go mod init aut-api && go build -o /aut-api .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    stress-ng iproute2 && rm -rf /var/lib/apt/lists/*
COPY --from=build /aut-api /aut-api
EXPOSE 8080
CMD ["/aut-api"]
