# syntax=docker/dockerfile-upstream:1.4.3

# postfix image: static Go agent (no Python runtime).
FROM golang:1.26-alpine AS build
ENV GOPROXY=https://goproxy.cn,direct CGO_ENABLED=0
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/internal/agent ./internal/agent
COPY backend/internal/oletools ./internal/oletools
COPY backend/cmd/mailstack-agent ./cmd/mailstack-agent
RUN go build -trimpath -ldflags="-s -w" -o /out/mailstack-agent ./cmd/mailstack-agent

FROM alpine:3.21

ARG VERSION=local
LABEL version=$VERSION
ARG APK_MIRROR="mirrors.aliyun.com"

RUN set -euxo pipefail \
  ; sed -i "s|dl-cdn.alpinelinux.org|${APK_MIRROR}|g" /etc/apk/repositories \
  ; apk add --no-cache cyrus-sasl-login postfix postfix-pcre logrotate

COPY --from=build /out/mailstack-agent /mailstack-agent

RUN echo $VERSION >/version

#EXPOSE 25/tcp 10025/tcp
HEALTHCHECK --start-period=30s CMD ! /usr/libexec/postfix/master -t

VOLUME ["/queue"]

CMD ["/mailstack-agent", "postfix"]
