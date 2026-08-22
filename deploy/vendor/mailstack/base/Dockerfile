# syntax=docker/dockerfile-upstream:1.4.3

# mailez base image: the shared Python runtime used by the mail-stack
# components (nginx/dovecot/postfix/rspamd configuration agents).
FROM alpine:3.21 as system

ENV TZ=Etc/UTC LANG=C.UTF-8

ARG MAILEZ_UID=1000
ARG MAILEZ_GID=1000

# China-friendly mirrors; override with --build-arg if needed.
ARG APK_MIRROR="mirrors.aliyun.com"
ARG PIP_INDEX_URL="https://mirrors.aliyun.com/pypi/simple/"

ENV PIP_INDEX_URL=${PIP_INDEX_URL}

RUN set -euxo pipefail \
  ; sed -i "s|dl-cdn.alpinelinux.org|${APK_MIRROR}|g" /etc/apk/repositories \
  ; addgroup -Sg ${MAILEZ_GID} mailez \
  ; adduser -Sg ${MAILEZ_UID} -G mailez -h /app -g "mailez app" -s /bin/bash mailez \
  ; apk add --no-cache bash ca-certificates curl python3 tzdata

WORKDIR /app

CMD /bin/bash

# build virtual env (intermediate)
FROM system as build

ENV VIRTUAL_ENV=/app/venv

COPY requirements-build.txt ./

RUN set -euxo pipefail \
  ; apk add --no-cache py3-pip \
  ; python3 -m venv ${VIRTUAL_ENV} \
  ; ${VIRTUAL_ENV}/bin/pip install --no-cache-dir -r requirements-build.txt \
  ; apk del -r py3-pip \
  ; rm -f /tmp/*.pem

COPY requirements-prod.txt ./
COPY libs/ libs/

ENV PATH="${VIRTUAL_ENV}/bin:${PATH}"

RUN set -euxo pipefail \
  ; apk add --virtual .build-deps build-base gcc libffi-dev python3-dev mariadb-dev \
  ; pip install --no-cache-dir -r requirements-prod.txt \
  ; apk del .build-deps \
  ; rm -rf /root/.cache

# base mailez image
FROM system

COPY --from=build /app/venv/ /app/venv/

ENV \
  VIRTUAL_ENV=/app/venv \
  PATH="/app/venv/bin:${PATH}" \
  ADMIN_ADDRESS="admin" \
  FRONT_ADDRESS="front" \
  FTS_ATTACHMENTS_ADDRESS="tika" \
  SMTP_ADDRESS="smtp" \
  IMAP_ADDRESS="imap" \
  OLETOOLS_ADDRESS="oletools" \
  REDIS_ADDRESS="redis" \
  ANTIVIRUS_ADDRESS="antivirus" \
  ANTISPAM_ADDRESS="antispam" \
  WEBMAIL_ADDRESS="webmail" \
  WEBDAV_ADDRESS="webdav"
