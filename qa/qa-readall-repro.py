#!/usr/bin/env python3
r"""Reproduce POST /mail/read-all on a mid-sized mailbox with full error text.

usage: qa-readall-repro.py <api_base> <imap_host> <imap_port> <count> [rnd]
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


def main():
    base, host, port, count = sys.argv[1:5]
    count = int(count)
    rnd = sys.argv[5] if len(sys.argv) > 5 else str(random.randint(100000, 999999))
    pw = "QaUser2026!"
    user = f"qa-rr-{rnd}@example.com"
    peer = f"qa-rrp-{rnd}@example.com"
    jar = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))

    def call(method, path, body=None, timeout=300, verbose=False):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(base + path, data=data, method=method,
                                     headers={"Content-Type": "application/json"} if data else {})
        t0 = time.time()
        try:
            with opener.open(req, timeout=timeout) as r:
                return r.status, r.read()[:120], round((time.time() - t0) * 1000), ""
        except urllib.error.HTTPError as e:
            return e.code, e.read()[:200], round((time.time() - t0) * 1000), ""
        except Exception as e:  # noqa: BLE001
            return 0, b"", round((time.time() - t0) * 1000), f"{type(e).__name__}: {e}"

    admin = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

    def admin_call(method, path, body=None, timeout=300):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(base + path, data=data, method=method,
                                     headers={"Content-Type": "application/json"} if data else {})
        try:
            with admin.open(req, timeout=timeout) as r:
                return r.status
        except urllib.error.HTTPError as e:
            return e.code

    print("admin login:", admin_call("POST", "/sso/login", {"email": "admin@example.com", "pw": "MailezDemo2026!"}))
    for u in (user, peer):
        print("mk", u, admin_call("POST", "/users", {"email": u, "password": pw, "display_name": "readall repro"}))

    ctx = ssl._create_unverified_context()
    m = imaplib.IMAP4(host, int(port))
    m.starttls(ctx)
    m.login(user, pw)
    m.select("INBOX")
    t0 = time.time()
    for i in range(count):
        raw = (f"From: {peer}\r\nTo: {user}\r\nSubject: rr-{rnd}-{i:04d}\r\n"
               f"Message-ID: <rr-{i}-{time.time_ns()}@qa>\r\n"
               "Content-Type: text/plain; charset=utf-8\r\n\r\nrepro body\r\n").encode()
        m.append("INBOX", None, imaplib.Time2Internaldate(time.time()), raw)
    print(f"seeded {count} in {time.time() - t0:.0f}s")
    m.logout()

    print("login as user:", call("POST", "/sso/login", {"email": user, "pw": pw})[0])
    print("unread before:", call("GET", "/mail/unseen")[0], call("GET", "/mail/unseen")[1])
    st, body, ms, err = call("POST", "/mail/read-all", {"folder": "Inbox"}, timeout=300)
    print(f"read-all: status={st} {ms}ms body={body} err={err}")
    print("unread after:", call("GET", "/mail/unseen")[0], call("GET", "/mail/unseen")[1])
    print("cleanup:", admin_call("DELETE", f"/users/{user}"), admin_call("DELETE", f"/users/{peer}"))


if __name__ == "__main__":
    main()
