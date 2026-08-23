# syntax=docker/dockerfile-upstream:1.4.3

# Build the static mailstack-agent for the resolver.
FROM golang:1.26-alpine AS build
ENV GOPROXY=https://goproxy.cn,direct CGO_ENABLED=0
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/internal/agent ./internal/agent
COPY backend/internal/macro ./internal/macro
COPY backend/cmd/mailstack-agent ./cmd/mailstack-agent
RUN go build -trimpath -ldflags="-s -w" -o /out/mailstack-agent ./cmd/mailstack-agent

# resolver image (single static agent binary)
FROM alpine:3.21

ARG VERSION=local
LABEL version=$VERSION
ARG APK_MIRROR="mirrors.aliyun.com"

RUN set -euxo pipefail \
  ; sed -i "s|dl-cdn.alpinelinux.org|${APK_MIRROR}|g" /etc/apk/repositories \
  ; apk add --no-cache ca-certificates curl bind-tools unbound \
  ; curl -so /etc/unbound/root.hints https://www.internic.net/domain/named.cache \
  ; chown root:unbound /etc/unbound \
  ; chmod 775 /etc/unbound \
  ; /usr/sbin/unbound-anchor -a /etc/unbound/trusted-key.key || true

COPY --from=build /out/mailstack-agent /mailstack-agent

RUN echo $VERSION >/version

#EXPOSE 53/udp 53/tcp
HEALTHCHECK CMD dig @127.0.0.1 || exit 1

CMD ["/mailstack-agent", "unbound"]
