# syntax=docker/dockerfile-upstream:1.4.3

FROM golang:1.26-alpine AS build
ENV CGO_ENABLED=0 GOPROXY=https://goproxy.cn,direct
WORKDIR /src
COPY . .
# Vendor verification key for technical-service entitlements (base64 DER
# SubjectPublicKeyInfo). Official release builds inject it via
# docker-bake.hcl from repo secrets; empty keeps the source-default key,
# which the server refuses to enforce entitlements on at startup (see
# internal/service.Load).
ARG MAILEZ_SERVICE_PUBKEY=""
RUN [ -n "$MAILEZ_SERVICE_PUBKEY" ] && SKEYS="-X mailez/backend/internal/service.publicKeyB64=$MAILEZ_SERVICE_PUBKEY" || SKEYS=""; \
    go build $SKEYS -o /out/mailez ./cmd/server \
 && go build $SKEYS -o /out/mailez-seed ./cmd/seed

FROM alpine:3.21
RUN adduser -S -u 1000 mailez
COPY --from=build /out/mailez /usr/local/bin/mailez
# One-shot admin provisioning for fresh deployments:
#   docker compose exec backend mailez-seed
COPY --from=build /out/mailez-seed /usr/local/bin/mailez-seed
USER mailez
EXPOSE 8081
ENTRYPOINT ["mailez"]
