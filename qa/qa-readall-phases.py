#!/usr/bin/env python3
r"""Time each phase the backend's read-all performs, at a size where it hangs.

usage: qa-readall-phases.py <api_base> <imap_host> <imap_port> <count> [rnd]
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
    user = f"qa-ph-{rnd}@example.com"
    peer = f"qa-php-{rnd}@example.com"
    admin = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

    def admin_call(method, path, body=None):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(base + path, data=data, method=method,
                                     headers={"Content-Type": "application/json"} if data else {})
        try:
            with admin.open(req, timeout=120) as r:
                return r.status
        except urllib.error.HTTPError as e:
            return e.code

    print("admin login:", admin_call("POST", "/sso/login", {"email": "admin@example.com", "pw": "MailezDemo2026!"}))
    for u in (user, peer):
        admin_call("POST", "/users", {"email": u, "password": pw, "display_name": "phases"})

    ctx = ssl._create_unverified_context()
    m = imaplib.IMAP4(host, int(port))
    m.starttls(ctx)
    m.login(user, pw)
    m.select("INBOX")
    t0 = time.time()
    for i in range(count):
        raw = (f"From: {peer}\r\nTo: {user}\r\nSubject: ph-{rnd}-{i:04d}\r\n"
               f"Message-ID: <ph-{i}-{time.time_ns()}@qa>\r\n"
               "Content-Type: text/plain; charset=utf-8\r\n\r\nbody\r\n").encode()
        m.append("INBOX", None, imaplib.Time2Internaldate(time.time()), raw)
    print(f"seeded {count} in {time.time() - t0:.0f}s")
    typ, data = m.select("INBOX")
    total = int(data[0])
    m.logout()

    m2 = imaplib.IMAP4(host, int(port))
    m2.starttls(ctx)
    m2.login(user, pw)
    m2.select("INBOX")

    def timed(label, fn):
        t = time.time()
        res = fn()
        print(f"  {label}: {(time.time() - t) * 1000:.0f}ms -> {res}", flush=True)

    timed("FETCH 1:N (UID FLAGS)", lambda: m2.fetch(f"1:{total}", "(UID FLAGS)")[0])
    timed("UID SEARCH ALL", lambda: m2.uid("search", None, "ALL")[0])

    uids = [str(i) for i in range(1, total + 1)]
    first = ",".join(uids[:500])
    rest = ",".join(uids[500:])
    timed("UID STORE first 500 +FLAGS (non-silent)", lambda: m2.uid("store", first, "+FLAGS", r"(\Seen)")[0])
    if rest:
        timed("UID STORE rest +FLAGS (non-silent)", lambda: m2.uid("store", rest, "+FLAGS", r"(\Seen)")[0])
    timed("UID STORE 1:N +FLAGS (range, non-silent)", lambda: m2.uid("store", f"1:{total}", "+FLAGS", r"(\Seen)")[0])
    timed("UID STORE 1:N +FLAGS.SILENT (range)", lambda: m2.uid("store", f"1:{total}", "+FLAGS.SILENT", r"(\Seen)")[0])
    m2.uid("store", f"1:{total}", "-FLAGS", r"(\Seen)")
    m2.logout()

    print("cleanup:", admin_call("DELETE", f"/users/{user}"), admin_call("DELETE", f"/users/{peer}"))


if __name__ == "__main__":
    main()
