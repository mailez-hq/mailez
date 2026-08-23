# syntax=docker/dockerfile-upstream:1.4.3

# Build the static mailstack-agent (M2 nginx agent, replaces start.py +
# config.py + letsencrypt.py).
FROM golang:1.26-alpine AS build
ENV GOPROXY=https://goproxy.cn,direct CGO_ENABLED=0
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/internal/agent ./internal/agent
COPY backend/cmd/mailstack-agent ./cmd/mailstack-agent
RUN go build -trimpath -ldflags="-s -w" -o /out/mailstack-agent ./cmd/mailstack-agent

# nginx image. certbot is still the Python ACME client; TODO(M2b): replace
# with an embedded lego client to drop the Python dependency.
FROM alpine:3.21

ARG VERSION
LABEL version=$VERSION

RUN set -euxo pipefail \
  ; apk add --no-cache ca-certificates curl certbot nginx nginx-mod-http-brotli nginx-mod-mail openssl dovecot-lua dovecot-pigeonhole-plugin 'dovecot-lmtpd<2.4' dovecot-pop3d dovecot-submissiond \
  ; mkdir -p /etc/dovecot

COPY --from=build /out/mailstack-agent /mailstack-agent
# This Dockerfile is built with the repo root as context (see
# build-images.ps1), hence the vendored paths below.
COPY deploy/vendor/mailstack/nginx/conf/dhparam.pem /conf/dhparam.pem
COPY deploy/vendor/mailstack/nginx/static/ /static/

RUN set -euxo pipefail \
  ; gzip -k9 /static/*.ico /static/*.txt \
  ; chmod a+rX-w -R /static \
  ; echo $VERSION >/version

EXPOSE 80/tcp 443/tcp 110/tcp 143/tcp 465/tcp 587/tcp 993/tcp 995/tcp 25/tcp 4190/tcp
# EXPOSE 10025/tcp 10143/tcp 14190/tcp
HEALTHCHECK --start-period=60s CMD curl -m3 -skfLo /dev/null http://127.0.0.1:10204/health && kill -0 `cat /run/dovecot/master.pid`

VOLUME ["/certs", "/overrides"]

CMD ["/mailstack-agent", "nginx"]
