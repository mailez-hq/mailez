#!/usr/bin/env sh
# mailezine 层二冒烟：对运行中的引擎走真实 SMTP/IMAP 线。
# 用法： ./smoke-mailezine.sh [smtp_port] [imap_port]
#   SMTP_PORT  引擎 SMTP 收信端口（默认 1025，compose profile 内为 25）
#   IMAP_PORT  引擎 IMAP 端口（默认 1143，compose profile 内为 143）
#
# 依赖： swaks、nc（IMAP 冒烟可选）。
set -eu

SMTP_PORT="${1:-1025}"
IMAP_PORT="${2:-1143}"
USER="${SMOKE_USER:-alice@example.com}"
PASS="${SMOKE_PASS:-s3cret}"
FROM="${SMOKE_FROM:-sender@example.net}"
TO="${SMOKE_TO:-alice@example.com}"

if ! command -v swaks >/dev/null 2>&1; then
  echo "smoke: swaks not found (brew install swaks / apt install swaks)" >&2
  exit 2
fi

echo "==> SMTP 投递 (port $SMTP_PORT)"
swaks --server 127.0.0.1:"$SMTP_PORT" \
  --from "$FROM" --to "$TO" \
  --header "Subject: mailezine smoke $(date +%s)" \
  --body "smoke test body" \
  --quit-after QUIT >/dev/null

if command -v nc >/dev/null 2>&1; then
  echo "==> IMAP 登录 + SELECT + FETCH (port $IMAP_PORT)"
  {
    printf 'a1 LOGIN %s %s\r\n' "$USER" "$PASS"
    printf 'a2 SELECT INBOX\r\n'
    printf 'a3 FETCH 1 (FLAGS BODY.PEEK[HEADER.FIELDS (SUBJECT)])\r\n'
    printf 'a4 LOGOUT\r\n'
    sleep 1
  } | nc -q 2 127.0.0.1 "$IMAP_PORT" \
    | grep -E '^\* [0-9]+ FETCH|a2 OK|a4 OK' || true
  echo "==> done (IMAP 输出含 * N FETCH 即通过)"
else
  echo "==> IMAP 冒烟跳过：未找到 nc（可用 dovecot imaptest 或 mailezine 集成测试替代）"
fi
