# syntax=docker/dockerfile-upstream:1.4.3

# Build the static mailstack-agent (dovecot agent).
FROM golang:1.26-alpine AS build
ENV GOPROXY=https://goproxy.cn,direct CGO_ENABLED=0
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/internal/agent ./internal/agent
COPY backend/internal/macro ./internal/macro
COPY backend/cmd/mailstack-agent ./cmd/mailstack-agent
RUN go build -trimpath -ldflags="-s -w" -o /out/mailstack-agent ./cmd/mailstack-agent

# dovecot image (single static agent binary)
FROM alpine:3.21

ARG VERSION
LABEL version=$VERSION
ARG APK_MIRROR="mirrors.aliyun.com"

RUN set -euxo pipefail \
  ; sed -i "s|dl-cdn.alpinelinux.org|${APK_MIRROR}|g" /etc/apk/repositories \
  ; apk add --no-cache 'dovecot<2.4' dovecot-lmtpd dovecot-pigeonhole-plugin dovecot-pop3d dovecot-submissiond rspamd-client dovecot-fts-flatcurve \
  ; mkdir -p /var/lib/dovecot /etc/dovecot /conf/bin

COPY --from=build /out/mailstack-agent /mailstack-agent
# This Dockerfile is built with the repo root as context.
COPY deploy/vendor/mailstack/dovecot/conf/ /conf/

RUN echo $VERSION >/version

# EXPOSE 110/tcp 143/tcp 993/tcp 4190/tcp 2525/tcp
HEALTHCHECK CMD kill -0 `cat /run/dovecot/master.pid`

VOLUME ["/mail"]

CMD ["/mailstack-agent", "dovecot"]
