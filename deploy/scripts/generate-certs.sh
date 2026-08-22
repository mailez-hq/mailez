#!/usr/bin/env sh
# Generates a self-signed certificate for local TLS testing.
# Usage:  ./generate-certs.sh [hostname]
# Output: deploy/certs/cert.pem + deploy/certs/key.pem (gitignored)

HOSTNAME="${1:-mail.example.com}"
DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)/certs"
mkdir -p "$DIR"

openssl req -x509 -newkey rsa:2048 \
  -keyout "$DIR/key.pem" -out "$DIR/cert.pem" -days 365 -nodes \
  -subj "/CN=$HOSTNAME" \
  -addext "subjectAltName=DNS:$HOSTNAME,DNS:localhost,IP:127.0.0.1"

echo "Wrote $DIR/cert.pem and $DIR/key.pem (self-signed, dev only)."
