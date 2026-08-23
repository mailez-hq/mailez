# syntax=docker/dockerfile-upstream:1.4.3

# macro-scanner image: static Go OLEFY/1.0-compatible scanner.
FROM golang:1.26-alpine AS build
ENV GOPROXY=https://goproxy.cn,direct CGO_ENABLED=0
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/internal/agent ./internal/agent
COPY backend/internal/macro ./internal/macro
COPY backend/cmd/mailez-agent ./cmd/mailez-agent
RUN go build -trimpath -ldflags="-s -w" -o /out/mailez-agent ./cmd/mailez-agent

FROM alpine:3.21

ARG VERSION=local
LABEL version=$VERSION
ARG APK_MIRROR="mirrors.aliyun.com"

RUN set -euxo pipefail \
  ; sed -i "s|dl-cdn.alpinelinux.org|${APK_MIRROR}|g" /etc/apk/repositories \
  ; apk add --no-cache netcat-openbsd ca-certificates

COPY --from=build /out/mailez-agent /mailez-agent

RUN echo $VERSION >/version

HEALTHCHECK --start-period=60s CMD echo PING|nc -q1 127.0.0.1 11343|grep "PONG"
#EXPOSE 11343/tcp

USER nobody:nobody

ENV \
    MAILEZ_SCANNER_BINDADDRESS="" \
    MAILEZ_SCANNER_BINDPORT="11343" \
    MAILEZ_SCANNER_MINLENGTH="300"

CMD ["/mailez-agent", "macro-scanner"]
