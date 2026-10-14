# Mailez v1.0.2 Release Notes

**Release Date:** October 14, 2026

This release introduces multi-signature support with organization footers, refines the webmail navigation chrome, and delivers critical fixes across installation, authentication, session management, ban attribution, and MIME processing.

## Highlights

### Multi-Signature Support & Organization Footer

Composing now supports multiple rich-text signatures per account (HTML with plain-text fallback), and administrators can publish an organization-wide footer from the admin console. The engine appends the footer to outbound mail when enabled for the sender's domain.

### Navigation Chrome Refinement

The admin console entry moved from the sidebar footer's overflow menu to the header (top right, admins only), and the sidebar footer shows filter rules as a direct icon — eliminating the "…" overflow menu entirely for a cleaner, more mainstream webmail experience.

### Installation & Seed Hardening

- `install.sh` now fails fast when seeding the admin account fails instead of reporting success with a warning that left a running stack with no admin
- Passkey registration fixed: user handle encoding switched from UTF-8 string to base64url as the WebAuthn spec requires, eliminating `atob()` failures on non-UTF-8 bytes
- Image tag persistence fixed: `MAILEZ_IMAGE_TAG` is now written into the generated env file even when commented out in the template, preventing fallback to `:latest`
- Admin seed panic resolved: `admin@<MAILEZ_DOMAIN>` is now constructed correctly regardless of domain length, with explicit `MAILEZ_ADMIN_EMAIL` validation

### Authentication & Session Security

- IMAP toggle enforcement hardened: port 143 exemption removed, so disabling IMAP takes effect immediately across all ports (not just 993)
- Auth cache scoped: real credentials (passwords/app tokens) always consult the control plane; only webmail session tokens are memoized, so operator changes apply on next login
- Session disconnect on disable: disabling an account or rotating credentials now drops live IMAP sessions via the management API, including AD/LDAP sync disables
- Ban engine attribution fixed: unattributable sources (empty IP) no longer create global bans that lock out all mail protocols; the engine now presents the connecting client's address on auth calls

### MIME Processing

Messages with nested `multipart/alternative` inside `multipart/mixed` — the most common layout — now render correctly. The extractor descends the full tree, honoring each part's transfer encoding and charset.

## Full Changelog

### Added

- Composing supports multiple rich-text signatures: an account keeps a list of signatures (HTML with a plain-text fallback) and the compose panel picks one per message. Administrators can also publish an organization-wide footer from the admin console; the engine appends it, when enabled for the sender's domain, to outbound mail

### Changed

- The webmail navigation chrome: the admin console entry moved from the sidebar footer's overflow menu to the header (top right, admins only), and the sidebar footer shows filter rules as a direct icon in the icon row, so the "…" overflow menu is gone

### Fixed

- Web push missed the first message after a subscription or a restart: the notifier primes its baseline from the account's first unseen read, so mail that landed before that read was folded into the baseline and never pushed. A delivery receipt now announces its Inbox copies directly, without waiting for the unseen count to agree. Junk copies stay quiet
- Forwarding dropped HTML formatting and attachments: the compose window opened with a plain-text quote and no files even when the original had rich HTML and attachments. Both forward entry points (`forward()` and `forwardFrom()`) now prefer the original `html_body` (falling back to plain text) and carry over the source message's attachments, mirroring how `editDraft()` restores drafts
- `install.sh` reported success when seeding the admin account failed: the failure was downgraded to a `seed failed (maybe already seeded)` warning, which left a running stack with no admin account at all. A failed seed is now a failed install, with the command to re-run once the cause is fixed
- Passkey registration failed before a credential was ever created, with `Failed to execute 'atob' on 'Window': The string to be decoded contains characters outside of the Latin1 range`. The ceremony was configured with `EncodeUserIDAsString`, which puts the user handle into a JSON string, while `WebAuthnID()` returns the raw 32 bytes of `sha256(email)`: `encoding/json` replaced every byte that is not valid UTF-8 with `U+FFFD` and the webmail's `atob()` rejected the result. The handle now leaves as the base64url string the specification asks for (go-webauthn's default), which is what the webmail decodes
- `install.sh` dropped the image tag: it rewrote `MAILEZ_IMAGE_TAG` in place, but the shipped `mailez.env.example` keeps that key commented out, so the substitution matched nothing, the generated `deploy/mailez.env` carried no tag at all and the stack fell back to pulling `:latest`. `--tag` and the interactive prompt now land in the file (the closing summary also prints the admin address as `admin@<MAILEZ_DOMAIN>`)
- `mailez-seed` panicked with `slice bounds out of range [:-1]` on a deployment whose `MAILEZ_DOMAIN` was as long as the default `admin@example.com`: the local part was cut off the email by subtracting the domain length, so equal lengths meant a negative bound (and any other mismatch wrote a wrong local part in silence). The account is now `admin@<MAILEZ_DOMAIN>` by default, and an explicit `MAILEZ_ADMIN_EMAIL` outside that domain stops with a message naming both values
- The per-user "Enable IMAP" toggle was silently ineffective on port 143: the mail auth endpoint exempted every credential that arrived on a webmail port, and 143 is both webmail's internal IMAP hop and the port the engine publishes to the internet, so a mailbox password or an app token logged in with a full IMAP client even after the operator had disabled IMAP for that account (the toggle only held on the unadvertised implicit-TLS port 993). The exemption now covers nothing but webmail's own short-lived `token-` session credential, and no longer consults the listener port at all — the engine serves webmail and the internet on the same one, so the port said nothing about who was connecting
- The engine's authentication cache replayed a success across protocols: its key was the credential pair alone, and every protocol shares one cache, so a single authenticated SMTP submission with an account's password let the next IMAP login through without the control plane ever being asked -- re-enabling IMAP for an account whose `enable_imap` was off. Real client credentials (passwords and app tokens) now always go to the control plane, and only webmail's session token — the credential the pooled IMAP/SMTP dials re-present — is memoised, so an operator's change also applies to the next login instead of one TTL later
- Disabling an account, turning IMAP/POP3 off or rotating a password left already-open IMAP sessions serving mail, because authorization is decided at login: the control plane now tells the engine (`POST /v1/accounts/{email}/disconnect` on the management API) to drop the account's live sessions on those changes, including when AD/LDAP sync disables an account that left the directory. The call fans out to every address the engine service resolves to, because each replica keeps its own session table and a single call would only reach one of them
- A burst of failed logins from an unattributable source locked out every mail-protocol login. The ban engine keys failure counters and bans on the source address, and a request with no resolvable address — the engine's own service-to-service auth calls among them — was recorded as the empty address; that ban row then matched every other unattributed request, so twenty failures answered every IMAP/SMTP/POP3 login with LIMIT (webmail showed a service error on every folder) and the escalation ladder kept the ban alive while clients kept retrying. Unattributable sources are no longer ban subjects, and the engine now presents the connecting client's address on auth calls, so bans attribute to real sources again
- Messages whose MIME tree nests `multipart/alternative` inside `multipart/mixed` — the layout most mailers emit — rendered with a blank body: body extraction walked a single level of multipart, so the nested text and HTML parts were never found. The extractor now descends the whole tree, honouring each part's own transfer encoding and charset; attachment, calendar and charset handling are unchanged

## Upgrade Notes

No database migrations or breaking changes. Existing deployments will pick up the new behavior automatically on next restart.

To use the organization footer feature:
1. Navigate to Admin Console → Settings → Organization Footer
2. Publish your footer content (HTML supported)
3. Enable it for specific domains as needed

To configure multi-signatures:
1. Go to Webmail Settings → Signatures
2. Add multiple signatures with rich-text editing
3. Select the default signature per account

## Contributors

Thanks to @FengHuangYuan for contributing the forward HTML+attachments fix (PR #8).
