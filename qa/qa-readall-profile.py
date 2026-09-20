#!/usr/bin/env python3
r"""Where does POST /mail/read-all spend its time?

Seeds a mailbox, then times (a) the engine's own UID FETCH/UID STORE over the
same UID set and (b) the webmail API call, so the slow layer is unambiguous.

usage: qa-readall-profile.py <api_base> <imap_host> <imap_port> <count> [rnd]
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
        t0 = time.time()
        try:
            with op.open(req, timeout=timeout) as r:
                return r.status, r.read()[:120], round((time.time() - t0) * 1000)
        except urllib.error.HTTPError as e:
            return e.code, e.read()[:120], round((time.time() - t0) * 1000)
        except Exception as e:  # noqa: BLE001
            return 0, str(e)[:120], round((time.time() - t0) * 1000)
    return call


def main():
    base, host, port, count = sys.argv[1:5]
    count = int(count)
    rnd = sys.argv[5] if len(sys.argv) > 5 else str(random.randint(100000, 999999))
    pw = "QaUser2026!"
    user = f"qa-ra-{rnd}@example.com"
    peer = f"qa-rap-{rnd}@example.com"

    admin = api(base, http.cookiejar.CookieJar())
    print("admin login:", admin("POST", "/sso/login", {"email": "admin@example.com", "pw": "MailezDemo2026!"})[0])
    for u in (user, peer):
        print("mk", u, admin("POST", "/users", {"email": u, "password": pw, "display_name": "RA profile"})[0])

    ctx = ssl._create_unverified_context()
    m = imaplib.IMAP4(host, int(port))
    m.starttls(ctx)
    m.login(user, pw)
    m.select("INBOX")
    t0 = time.time()
    for i in range(count):
        raw = (f"From: {peer}\r\nTo: {user}\r\nSubject: ra-{rnd}-{i:04d}\r\n"
               f"Message-ID: <ra-{i}-{time.time_ns()}@qa>\r\n"
               "Content-Type: text/plain; charset=utf-8\r\n\r\nprofile body\r\n").encode()
        m.append("INBOX", None, imaplib.Time2Internaldate(time.time()), raw)
    print(f"seeded {count} in {time.time() - t0:.0f}s")

    def timed_imap(label, fn):
        t = time.time()
        res = fn()
        print(f"  {label}: {round((time.time() - t) * 1000)}ms -> {res}")
        return res

    typ, data = m.select("INBOX")
    total = int(data[0])
    m.logout()

    m = imaplib.IMAP4(host, int(port))
    m.starttls(ctx)
    m.login(user, pw)
    m.select("INBOX")
    timed_imap("UID FETCH 1:* (FLAGS)", lambda: m.uid("fetch", f"1:{total}", "(FLAGS)")[0])
    timed_imap("UID STORE 1:* +FLAGS Seen", lambda: m.uid("store", f"1:{total}", "+FLAGS", r"(\Seen)")[0])
    m.logout()

    ua = api(base, http.cookiejar.CookieJar())
    ua("POST", "/sso/login", {"email": user, "pw": pw})
    print("webmail list:", ua("GET", "/mail/messages?folder=Inbox&page=0")[0])
    m = imaplib.IMAP4(host, int(port))
    m.starttls(ctx)
    m.login(user, pw)
    m.select("INBOX")
    timed_imap("UID STORE 1:* -FLAGS Seen (reset)", lambda: m.uid("store", f"1:{total}", "-FLAGS", r"(\Seen)")[0])
    m.logout()
    st, body, ms = ua("POST", "/mail/read-all", {"folder": "Inbox"})
    print(f"API POST /mail/read-all: status={st} {ms}ms body={body[:80]}")

    print("cleanup:", admin("DELETE", f"/users/{user}")[0], admin("DELETE", f"/users/{peer}")[0])


if __name__ == "__main__":
    main()
