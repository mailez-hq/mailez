# Changelog

All notable changes to mailez are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and adheres to
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Web Push notifications: VAPID endpoint, subscription API, background
  notifier polling unseen counts for subscribed users
- Prometheus metrics on a dedicated port (`MAILEZ_METRICS_ADDR`, default :9090)
- Versioned database migrations (`schema_migrations` table)
- Login rate limiting (per IP and per account) and TOTP brute-force protection
- Frontend engineering scaffold: ESLint, Prettier, Vitest and CI jobs for
  webmail and admin
- Shared API contract types in `frontend/packages/types`
- Exchange ActiveSync (EAS) server at `/Microsoft-Server-ActiveSync` plus
  autodiscover: WBXML codec, device registry, FolderSync, incremental mail
  Sync (read/flag/delete/move), Ping long-poll, SendMail/SmartReply/
  SmartForward, Search (mailbox + GAL), Settings, ItemOperations, meeting
  invitations and ResolveRecipients; configure iOS/Outlook with the mailbox
  password or an app token
- Meeting invitations (iTIP): parse REQUEST/REPLY/CANCEL, one-click
  accept/decline/tentative from the mail reader and the calendar, send new
  invitations with attendees
- Outbound DLP and approval workflow: keyword/regex rules, hold-for-approval,
  approver console and expiry auto-reject (Coremail-style 审批)
- Compliance email archive: engine-side capture of inbound/outbound mail,
  retention policies, metadata search, review notes, .eml download and mbox
  export (Coremail-style 归档)
- AD/LDAP directory integration: authentication fallback, organization
  address book with department tree, group mailboxes with delivery-time
  nested expansion, and account lifecycle sync
- Attachment full-text search in mailezine (PDF/OOXML/ODF/text extraction
  with Tika fallback) for the mailbox search
- Internal `/stack` API authentication via `MAILEZ_STACK_SECRET`
  (`X-Stack-Secret` header) used by the mail agent and the mailezine engine

### Changed

- Backend reorganized from a flat `api` package into domain packages
  (`user`, `domain`, `alias`, `mailbox`, `compose`, `contacts`, `sieve`,
  `admin`, `fetch`, `push`, `ai`, `stack`); `mail.Client` is now behind the
  `mail.Gateway` interface so handlers are testable
- Mail-stack service renamed `mail-store` → `mail-keeper`; compose project
  name pinned to `mailez` (containers `mailez-*`)
- HTTP hardening: request/response timeouts, 64 MiB body limit, CORS
  allow-list, request-id and access logging
- Error responses no longer leak internals; 5xx map to generic messages
- Mail HTML is sanitized server-side (bluemonday) and client-side (DOMPurify)
- Graceful shutdown on SIGINT/SIGTERM; background workers cancel cleanly

### Fixed

- Multipart message builder emitted a duplicate boundary, corrupting the
  first body part when attachments were present
- Attachments larger than 4 MiB were rejected by the default HTTP body limit
- External POP3/IMAP fetch disabled TLS verification (now verified by default;
  `FETCH_INSECURE` opts out)

### Security

- Email HTML rendering was vulnerable to stored XSS; sanitized on both sides
- Web login had no brute-force protection (rate limited)
- Session cookies were not marked Secure; `COOKIE_SECURE` now controls it
- Default `SECRET_KEY` is rejected in production mode
