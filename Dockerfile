FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /currencies ./cmd/server

FROM alpine:3.21
RUN adduser -D -u 10001 appuser
COPY --from=build /currencies /usr/local/bin/currencies
USER appuser
EXPOSE 8080 8081 8082
ENTRYPOINT ["currencies"]
