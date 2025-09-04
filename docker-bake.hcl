# docker-bake.hcl — single source of truth for building all mailez images
# with `docker buildx bake`.
#
# Targets:
#   backend, admin, webmail            control-plane and web apps
#   nginx, rspamd, unbound,            infra images
#   macro-scanner, engine-lb
#   mailezine                          engine image; builds from the separate
#                                      mailezine repo, resolved via
#                                      MAILEZINE_CONTEXT
#
# Examples:
#   docker buildx bake                                  # all images, tag :local
#   docker buildx bake mailezine                        # engine only
#   VERSION=v1.2.3 APK_MIRROR=dl-cdn.alpinelinux.org \
#     PLATFORMS=linux/amd64,linux/arm64 \
#     docker buildx bake --push

variable "VERSION" {
  # Image tag (and Dockerfile VERSION args). "local" matches the compose
  # default for local builds; release CI overrides it with the git tag.
  default = "local"
}

variable "REGISTRY" {
  default = "ghcr.io/mailez-hq"
}

variable "PLATFORMS" {
  default = "linux/amd64"
}

variable "APK_MIRROR" {
  # Alpine apk mirror baked into the infra images. Local builds keep the CN
  # mirror (the Dockerfiles' own default); release CI overrides it with
  # dl-cdn.alpinelinux.org.
  default = "mirrors.aliyun.com"
}

variable "MAILEZINE_CONTEXT" {
  # Build context for the mailezine engine (relative to this file). CI
  # checks out mailez-hq/mailezine next to this repo and sets
  # MAILEZINE_CONTEXT=../mailezine.
  default = "../mailezine"
}

group "default" {
  targets = [
    "backend",
    "admin",
    "webmail",
    "nginx",
    "rspamd",
    "unbound",
    "macro-scanner",
    "engine-lb",
  ]
}

group "mailezine" {
  targets = [
    "mailezine",
  ]
}

target "backend" {
  context = "."
  dockerfile = "backend/Dockerfile"
  tags = ["${REGISTRY}/mailez-backend:${VERSION}"]
  platforms = split(",", PLATFORMS)
}

target "admin" {
  context = "./frontend"
  dockerfile = "apps/admin/Dockerfile"
  tags = ["${REGISTRY}/mailez-admin:${VERSION}"]
  platforms = split(",", PLATFORMS)
}

target "webmail" {
  context = "./frontend"
  dockerfile = "apps/webmail/Dockerfile"
  tags = ["${REGISTRY}/mailez-webmail:${VERSION}"]
  platforms = split(",", PLATFORMS)
}

target "mailezine" {
  context = MAILEZINE_CONTEXT
  dockerfile = "Dockerfile"
  args = { VERSION = VERSION }
  tags = ["${REGISTRY}/mailez-mailezine:${VERSION}"]
  platforms = split(",", PLATFORMS)
}

target "nginx" {
  context = "."
  dockerfile = "deploy/images/gateway/Dockerfile"
  args = { VERSION = VERSION, APK_MIRROR = APK_MIRROR }
  tags = ["${REGISTRY}/mailez-nginx:${VERSION}"]
  platforms = split(",", PLATFORMS)
}

target "rspamd" {
  context = "."
  dockerfile = "deploy/images/mail-filter/Dockerfile"
  args = { VERSION = VERSION, APK_MIRROR = APK_MIRROR }
  tags = ["${REGISTRY}/mailez-rspamd:${VERSION}"]
  platforms = split(",", PLATFORMS)
}

target "unbound" {
  context = "."
  dockerfile = "deploy/images/resolver/Dockerfile"
  args = { VERSION = VERSION, APK_MIRROR = APK_MIRROR }
  tags = ["${REGISTRY}/mailez-unbound:${VERSION}"]
  platforms = split(",", PLATFORMS)
}

target "macro-scanner" {
  context = "."
  dockerfile = "deploy/images/macro-scanner/Dockerfile"
  args = { VERSION = VERSION, APK_MIRROR = APK_MIRROR }
  tags = ["${REGISTRY}/mailez-macro-scanner:${VERSION}"]
  platforms = split(",", PLATFORMS)
}

target "engine-lb" {
  context = "./deploy/images/engine-lb"
  dockerfile = "Dockerfile"
  tags = ["${REGISTRY}/mailez-engine-lb:${VERSION}"]
  platforms = split(",", PLATFORMS)
}
