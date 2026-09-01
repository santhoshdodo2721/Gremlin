FROM golang:1.22-alpine AS build
WORKDIR /src
COPY main.go .
RUN go mod init aut-api && go mod tidy && go build -o /aut-api .

FROM alpine:3.19
RUN apk add --no-cache stress-ng iproute2
COPY --from=build /aut-api /aut-api
EXPOSE 8080
CMD ["/aut-api"]
