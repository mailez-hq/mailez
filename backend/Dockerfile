# syntax=docker/dockerfile-upstream:1.4.3

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /out/mailez ./cmd/server

FROM alpine:3.21
RUN adduser -S -u 1000 mailez
COPY --from=build /out/mailez /usr/local/bin/mailez
USER mailez
EXPOSE 8081
ENTRYPOINT ["mailez"]
