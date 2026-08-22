# syntax=docker/dockerfile-upstream:1.4.3

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /out/mailess ./cmd/server

FROM alpine:3.21
RUN adduser -S -u 1000 mailess
COPY --from=build /out/mailess /usr/local/bin/mailess
USER mailess
EXPOSE 8081
ENTRYPOINT ["mailess"]
