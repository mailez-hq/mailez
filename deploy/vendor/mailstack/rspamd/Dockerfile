# syntax=docker/dockerfile-upstream:1.4.3

# rspamd image: static Go agent (no Python runtime).
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
  ; apk add --no-cache rspamd rspamd-controller rspamd-fuzzy rspamd-proxy curl \
  ; mkdir /run/rspamd \
  ; mkdir /overrides

COPY --from=build /out/mailstack-agent /mailstack-agent

RUN echo $VERSION >/version

#EXPOSE 11332/tcp 11334/tcp 11335/tcp
HEALTHCHECK --start-period=350s CMD curl -m3 -skfLo /dev/null http://localhost:11334/

VOLUME ["/var/lib/rspamd"]

CMD ["/mailstack-agent", "rspamd"]
