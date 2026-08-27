# syntax=docker/dockerfile-upstream:1.4.3

# Build the static mailez-agent (nginx gateway agent).
FROM golang:1.26-alpine AS build
ENV GOPROXY=https://goproxy.cn,direct CGO_ENABLED=0
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/internal/agent ./internal/agent
COPY backend/internal/macro ./internal/macro
COPY backend/cmd/mailez-agent ./cmd/mailez-agent
RUN go build -trimpath -ldflags="-s -w" -o /out/mailez-agent ./cmd/mailez-agent

# Unified gateway image for both engine editions (nginx + embedded dovecot
# login proxy). ACME is handled by the embedded lego client; the mailezine
# mode runs the same nginx with an HTTP-only config (the engine publishes
# the mail ports itself).
FROM alpine:3.21

ARG VERSION
LABEL version=$VERSION
ARG APK_MIRROR="mirrors.aliyun.com"

RUN set -euxo pipefail \
  ; sed -i "s|dl-cdn.alpinelinux.org|${APK_MIRROR}|g" /etc/apk/repositories \
  ; apk add --no-cache ca-certificates curl nginx nginx-mod-http-brotli nginx-mod-mail openssl dovecot-lua dovecot-pigeonhole-plugin 'dovecot-lmtpd<2.4' dovecot-pop3d dovecot-submissiond \
  ; mkdir -p /etc/dovecot

COPY --from=build /out/mailez-agent /mailez-agent
# This Dockerfile is built with the repo root as context (see
# build script), hence the vendored paths below.
COPY deploy/images/gateway/conf/dhparam.pem /conf/dhparam.pem
COPY deploy/images/gateway/static/ /static/

RUN set -euxo pipefail \
  ; gzip -k9 /static/*.ico /static/*.txt \
  ; chmod a+rX-w -R /static \
  ; echo $VERSION >/version

EXPOSE 80/tcp 443/tcp 110/tcp 143/tcp 465/tcp 587/tcp 993/tcp 995/tcp 25/tcp 4190/tcp
# EXPOSE 1587/tcp 1143/tcp 11490/tcp
# nginx's localhost health server is engine-agnostic (dovecot only runs in
# postdove mode, so its pid must not gate the health check).
HEALTHCHECK --start-period=60s CMD curl -m3 -skfLo /dev/null http://127.0.0.1:10204/health

VOLUME ["/certs", "/overrides"]

CMD ["/mailez-agent", "nginx"]
