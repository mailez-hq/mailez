# docker-bake.ee.hcl — enterprise edition bake targets (private tree only).
#
# Load together with docker-bake.hcl:
#   docker buildx bake -f docker-bake.hcl -f docker-bake.ee.hcl ee
#   docker buildx bake -f docker-bake.hcl -f docker-bake.ee.hcl ee mailezine --push
#
# This file is stripped from the community mirror (scripts/export-ce.sh):
# the CE tree has no EE targets and no vendor license-key variables.

variable "MAILEZ_LICENSE_PUBKEY" {
  # Vendor Ed25519 verification keys baked into EE release builds (base64
  # DER SPKI; `go run ./cmd/license genkey` prints a pair — the service
  # key shares it). Overridden by same-name environment variables; the
  # release workflow wires the repo secrets. Empty keeps the source-default
  # dev key, which the backend refuses to enforce MAILEZ_LICENSE_REQUIRED
  # on at startup — so official EE builds MUST set these.
  default = ""
}

variable "MAILEZ_SERVICE_PUBKEY" {
  default = ""
}

group "ee" {
  targets = [
    "backend-ee",
    "admin-ee",
    "webmail-ee",
  ]
}

group "mailezine" {
  # Overrides the CE file's ce-only "mailezine" group when both files are
  # loaded: the engine also ships an EE edition.
  targets = [
    "mailezine-ce",
    "mailezine-ee",
  ]
}

target "backend-ee" {
  context = "."
  dockerfile = "backend/Dockerfile"
  args = {
    MAILEZ_EDITION = "ee"
    MAILEZ_LICENSE_PUBKEY = MAILEZ_LICENSE_PUBKEY
    MAILEZ_SERVICE_PUBKEY = MAILEZ_SERVICE_PUBKEY
  }
  tags = ["${REGISTRY}/mailez-backend-ee:${VERSION}"]
  platforms = split(",", PLATFORMS)
}

target "admin-ee" {
  context = "./frontend"
  dockerfile = "apps/admin/Dockerfile"
  args = { MAILEZ_EDITION = "ee" }
  tags = ["${REGISTRY}/mailez-admin-ee:${VERSION}"]
  platforms = split(",", PLATFORMS)
}

target "webmail-ee" {
  context = "./frontend"
  dockerfile = "apps/webmail/Dockerfile"
  args = { MAILEZ_EDITION = "ee" }
  tags = ["${REGISTRY}/mailez-webmail-ee:${VERSION}"]
  platforms = split(",", PLATFORMS)
}

target "mailezine-ee" {
  context = MAILEZINE_CONTEXT
  dockerfile = "Dockerfile"
  args = { MAILEZ_EDITION = "ee", VERSION = VERSION }
  tags = ["${REGISTRY}/mailez-mailezine-ee:${VERSION}"]
  platforms = split(",", PLATFORMS)
}
