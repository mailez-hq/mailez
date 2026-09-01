# syntax=docker/dockerfile-upstream:1.4.3

FROM golang:1.26-alpine AS build
ENV CGO_ENABLED=0 GOPROXY=https://goproxy.cn,direct
WORKDIR /src
COPY . .
# MAILEZ_EDITION=ee compiles the enterprise control-plane features (EE-only
# routes and jobs, audit CSV export, OIDC SSO, LDAP sync, …) via the
# mailez_ee build tag; the default community build excludes internal/ee
# entirely. The enterprise compose passes MAILEZ_EDITION=ee.
ARG MAILEZ_EDITION=ce
RUN [ "$MAILEZ_EDITION" = "ee" ] && TAGS="-tags mailez_ee" || TAGS=""; \
    go build $TAGS -o /out/mailez ./cmd/server \
 && go build $TAGS -o /out/mailez-seed ./cmd/seed

FROM alpine:3.21
RUN adduser -S -u 1000 mailez
COPY --from=build /out/mailez /usr/local/bin/mailez
# One-shot admin provisioning for fresh deployments:
#   docker compose exec backend mailez-seed
COPY --from=build /out/mailez-seed /usr/local/bin/mailez-seed
USER mailez
EXPOSE 8081
ENTRYPOINT ["mailez"]
