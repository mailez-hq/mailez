# syntax=docker/dockerfile-upstream:1.4.3

# mailezine-mode gateway: caddy only (HTTP/ACME). The engine publishes the
# mail ports itself, so this image carries no nginx/dovecot at all.

# Build the static mailez-agent (shared by both gateway variants).
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

ARG VERSION
LABEL version=$VERSION
ARG APK_MIRROR="mirrors.aliyun.com"

RUN set -euxo pipefail \
  ; sed -i "s|dl-cdn.alpinelinux.org|${APK_MIRROR}|g" /etc/apk/repositories \
  ; apk add --no-cache ca-certificates curl caddy \
  ; mkdir -p /etc/caddy

COPY --from=build /out/mailez-agent /mailez-agent
COPY deploy/images/gateway/static/ /static/

RUN set -euxo pipefail \
  ; gzip -k9 /static/*.ico /static/*.txt \
  ; chmod a+rX-w -R /static \
  ; echo $VERSION >/version

EXPOSE 80/tcp 443/tcp
HEALTHCHECK --start-period=30s CMD curl -m3 -sfkLo /dev/null http://127.0.0.1/health

CMD ["/mailez-agent", "nginx"]
