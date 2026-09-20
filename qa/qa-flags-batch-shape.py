#!/usr/bin/env python3
r"""How should a bulk flag write be chunked?

Seeds a mailbox, then times the same UID set as one big STORE versus several
smaller ones, so the engine's batch size comes from measurements.

usage: qa-flags-batch-shape.py <api_base> <imap_host> <imap_port> <count> [rnd]
"""
import http.cookiejar
import imaplib
import json
import random
import ssl
import sys
import time
import urllib.error
import urllib.request


def api(base, jar):
    op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))

    def call(method, path, body=None, timeout=600):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(base + path, data=data, method=method,
                                     headers={"Content-Type": "application/json"} if data else {})
        try:
            with op.open(req, timeout=timeout) as r:
                return r.status, r.read()[:120]
        except urllib.error.HTTPError as e:
            return e.code, e.read()[:120]
        except Exception as e:  # noqa: BLE001
            return 0, str(e)[:120]
    return call


def main():
    base, host, port, count = sys.argv[1:5]
    count = int(count)
    rnd = sys.argv[5] if len(sys.argv) > 5 else str(random.randint(100000, 999999))
    pw = "QaUser2026!"
    user = f"qa-fs-{rnd}@example.com"
    peer = f"qa-fsp-{rnd}@example.com"

    admin = api(base, http.cookiejar.CookieJar())
    print("admin login:", admin("POST", "/sso/login", {"email": "admin@example.com", "pw": "MailezDemo2026!"})[0])
    for u in (user, peer):
        admin("POST", "/users", {"email": u, "password": pw, "display_name": "flag shape"})

    ctx = ssl._create_unverified_context()
    m = imaplib.IMAP4(host, int(port))
    m.starttls(ctx)
    m.login(user, pw)
    m.select("INBOX")
    t0 = time.time()
    for i in range(count):
        raw = (f"From: {peer}\r\nTo: {user}\r\nSubject: fs-{rnd}-{i:04d}\r\n"
               f"Message-ID: <fs-{i}-{time.time_ns()}@qa>\r\n"
               "Content-Type: text/plain; charset=utf-8\r\n\r\nshape body\r\n").encode()
        m.append("INBOX", None, imaplib.Time2Internaldate(time.time()), raw)
    print(f"seeded {count} in {time.time() - t0:.0f}s")
    typ, data = m.select("INBOX")
    total = int(data[0])
    m.logout()

    def store(chunk):
        m2 = imaplib.IMAP4(host, int(port))
        m2.starttls(ctx)
        m2.login(user, pw)
        m2.select("INBOX")
        t = time.time()
        for start in range(1, total + 1, chunk):
            end = min(start + chunk - 1, total)
            m2.uid("store", f"{start}:{end}", "+FLAGS", r"(\Seen)")
        dur = time.time() - t
        m2.uid("store", f"1:{total}", "-FLAGS", r"(\Seen)")
        m2.logout()
        return dur

    for chunk in (total, max(total // 2, 1), max(total // 6, 1), max(total // 30, 1)):
        dur = store(chunk)
        print(f"  +FLAGS in chunks of {chunk}: {dur * 1000:.0f}ms ({dur * 1000 / total:.1f}ms/message)")

    print("cleanup:", admin("DELETE", f"/users/{user}")[0], admin("DELETE", f"/users/{peer}")[0])


if __name__ == "__main__":
    main()
