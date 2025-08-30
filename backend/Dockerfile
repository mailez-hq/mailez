# syntax=docker/dockerfile-upstream:1.4.3

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /out/mailez ./cmd/server \
 && CGO_ENABLED=0 go build -o /out/mailez-seed ./cmd/seed

FROM alpine:3.21
RUN adduser -S -u 1000 mailez
COPY --from=build /out/mailez /usr/local/bin/mailez
# One-shot admin provisioning for fresh deployments:
#   docker compose exec backend mailez-seed
COPY --from=build /out/mailez-seed /usr/local/bin/mailez-seed
USER mailez
EXPOSE 8081
ENTRYPOINT ["mailez"]
