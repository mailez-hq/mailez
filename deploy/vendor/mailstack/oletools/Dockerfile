# syntax=docker/dockerfile-upstream:1.4.3

# oletools image: static Go OLEFY/1.0-compatible scanner.
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
  ; apk add --no-cache netcat-openbsd ca-certificates

COPY --from=build /out/mailstack-agent /mailstack-agent

RUN echo $VERSION >/version

HEALTHCHECK --start-period=60s CMD echo PING|nc -q1 127.0.0.1 11343|grep "PONG"
#EXPOSE 11343/tcp

USER nobody:nobody

ENV \
    OLEFY_BINDADDRESS="" \
    OLEFY_BINDPORT="11343" \
    OLEFY_MINLENGTH="300" \
    OLEFY_DEL_TMP="1" \
    OLEFY_DEL_TMP_FAILED="1"

CMD ["/mailstack-agent", "oletools"]
