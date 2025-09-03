# docker-bake.hcl — single source of truth for building all mailez images
# with `docker buildx bake` (replaces backend/cmd/build-images).
#
# Image matrix (9 components / 13 package names):
#   backend / admin / webmail / mailezine ship CE and EE variants. CE and EE
#   live under separate GHCR package names because GHCR access control is per
#   package, not per tag (CE packages public, EE packages private):
#     ghcr.io/mailez-hq/mailez-backend      vs mailez-backend-ee
#   nginx / rspamd / unbound / macro-scanner / engine-lb are edition-neutral.
#
# This file holds the CE targets; the EE targets and the vendor license-key
# variables live in docker-bake.ee.hcl (private tree, stripped from the
# community mirror).
#
# Groups:
#   default    = ce
#   ce         backend-ce, admin-ce, webmail-ce, nginx, rspamd, unbound,
#              macro-scanner, engine-lb
#   mailezine  mailezine-ce
#              (kept out of ce/ee on purpose: it builds from the separate
#              mailezine repo, resolved via MAILEZINE_CONTEXT; the EE bake
#              file re-adds mailezine-ee to this group)
#
# Examples:
#   docker buildx bake                                  # CE, tag :local
#   docker buildx bake ce mailezine                     # CE + mailezine
#   docker buildx bake -f docker-bake.hcl -f docker-bake.ee.hcl ee  # EE variants
#   VERSION=v1.2.3 APK_MIRROR=dl-cdn.alpinelinux.org \
#     PLATFORMS=linux/amd64,linux/arm64 \
#     docker buildx bake -f docker-bake.hcl -f docker-bake.ee.hcl ce ee mailezine --push

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
  targets = ["ce"]
}

group "ce" {
  targets = [
    "backend-ce",
    "admin-ce",
    "webmail-ce",
    "nginx",
    "rspamd",
    "unbound",
    "macro-scanner",
    "engine-lb",
  ]
}

group "mailezine" {
  targets = [
    "mailezine-ce",
  ]
}

target "backend-ce" {
  context = "."
  dockerfile = "backend/Dockerfile"
  args = { MAILEZ_EDITION = "ce" }
  tags = ["${REGISTRY}/mailez-backend:${VERSION}"]
  platforms = split(",", PLATFORMS)
}

target "admin-ce" {
  context = "./frontend"
  dockerfile = "apps/admin/Dockerfile"
  args = { MAILEZ_EDITION = "ce" }
  tags = ["${REGISTRY}/mailez-admin:${VERSION}"]
  platforms = split(",", PLATFORMS)
}

target "webmail-ce" {
  context = "./frontend"
  dockerfile = "apps/webmail/Dockerfile"
  args = { MAILEZ_EDITION = "ce" }
  tags = ["${REGISTRY}/mailez-webmail:${VERSION}"]
  platforms = split(",", PLATFORMS)
}

target "mailezine-ce" {
  context = MAILEZINE_CONTEXT
  dockerfile = "Dockerfile"
  args = { MAILEZ_EDITION = "ce", VERSION = VERSION }
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
