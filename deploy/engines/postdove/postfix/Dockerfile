# syntax=docker/dockerfile-upstream:1.4.3

# postfix image: static Go agent.
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
  ; apk add --no-cache cyrus-sasl-login postfix postfix-pcre logrotate

COPY --from=build /out/mailez-agent /mailez-agent

RUN echo $VERSION >/version

#EXPOSE 25/tcp 1587/tcp
HEALTHCHECK --start-period=30s CMD ! /usr/libexec/postfix/master -t

VOLUME ["/queue"]

CMD ["/mailez-agent", "postfix"]
