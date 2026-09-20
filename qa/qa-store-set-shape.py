#!/usr/bin/env python3
r"""Does the STORE cost depend on how the target set is written?

The backend sends UID STORE with an explicit list of UIDs; the earlier shape
test used sequence ranges. This compares both forms (and a mixed one) on the
same mailbox.

usage: qa-store-set-shape.py <api_base> <imap_host> <imap_port> <count> [rnd]
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
    user = f"qa-ss-{rnd}@example.com"
    peer = f"qa-ssp-{rnd}@example.com"
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
        admin_call("POST", "/users", {"email": u, "password": pw, "display_name": "store shape"})

    ctx = ssl._create_unverified_context()
    m = imaplib.IMAP4(host, int(port))
    m.starttls(ctx)
    m.login(user, pw)
    m.select("INBOX")
    t0 = time.time()
    for i in range(count):
        raw = (f"From: {peer}\r\nTo: {user}\r\nSubject: ss-{rnd}-{i:04d}\r\n"
               f"Message-ID: <ss-{i}-{time.time_ns()}@qa>\r\n"
               "Content-Type: text/plain; charset=utf-8\r\n\r\nbody\r\n").encode()
        m.append("INBOX", None, imaplib.Time2Internaldate(time.time()), raw)
    print(f"seeded {count} in {time.time() - t0:.0f}s")
    typ, data = m.select("INBOX")
    total = int(data[0])
    m.logout()

    def run(label, seq_arg, uid_arg):
        m2 = imaplib.IMAP4(host, int(port))
        m2.starttls(ctx)
        m2.login(user, pw)
        m2.select("INBOX")
        t = time.time()
        if uid_arg:
            m2.uid("store", uid_arg, "+FLAGS", r"(\Seen)")
            m2.uid("store", uid_arg, "-FLAGS", r"(\Seen)")
        else:
            m2.uid("store", seq_arg, "+FLAGS", r"(\Seen)")
            m2.uid("store", seq_arg, "-FLAGS", r"(\Seen)")
        dur = time.time() - t
        m2.logout()
        print(f"  {label}: {dur * 1000:.0f}ms")

    rng = f"1:{total}"
    uid_list = ",".join(str(i) for i in range(1, total + 1))
    uid_ranges = ",".join(f"{i}:{i + 9}" for i in range(1, total + 1, 10))
    run("UID STORE range 1:N (plus+minus)", rng, None)
    run("UID STORE individual uid list", None, uid_list)
    run("UID STORE uid ranges of 10", None, uid_ranges)

    print("cleanup:", admin_call("DELETE", f"/users/{user}"), admin_call("DELETE", f"/users/{peer}"))


if __name__ == "__main__":
    main()
