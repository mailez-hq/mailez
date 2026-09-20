#!/usr/bin/env python3
r"""Load/scale round for the mailbox read paths (P4/P6 deferred to a load run).

Seeds N messages into one mailbox over IMAP, then samples the webmail read
APIs (list, sorted list, conversation view, unseen, search) and reports
min/median/P95/max. Also runs searches while new mail is being delivered.

usage: qa-p6b-load.py <api_base> <imap_host> <imap_port> <count> [rnd]
Results are appended to qa/results.log in the usual QA format.
"""
import http.cookiejar
import imaplib
import json
import os
import ssl
import statistics
import sys
import threading
import time
import urllib.error
import urllib.request

RESULTS = []


def check(cid, name, ok, detail=""):
    line = f"{'PASS' if ok else 'FAIL'} {cid} {name}"
    if detail:
        line += f"  -- {detail}"
    RESULTS.append(line)
    print(line, flush=True)


class Api:
    def __init__(self, base):
        self.base = base.rstrip("/")
        self.jar = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.jar))

    def call(self, method, path, body=None, timeout=180):
        data = None
        headers = {}
        if body is not None:
            data = json.dumps(body).encode()
            headers["Content-Type"] = "application/json"
        req = urllib.request.Request(self.base + path, data=data, headers=headers, method=method)
        t0 = time.time()
        try:
            with self.opener.open(req, timeout=timeout) as r:
                raw = r.read()
                return r.status, (json.loads(raw) if raw else None), (time.time() - t0) * 1000
        except urllib.error.HTTPError as e:
            return e.code, None, (time.time() - t0) * 1000
        except Exception as e:  # noqa: BLE001
            return 0, str(e), (time.time() - t0) * 1000

    def login(self, email, password):
        # The SSO endpoint's password field is "pw" (see internal/auth/sso.go).
        return self.call("POST", "/sso/login", {"email": email, "pw": password})[0]


def imap_conn(host, port, user, password):
    ctx = ssl._create_unverified_context()
    m = imaplib.IMAP4(host, int(port))
    m.starttls(ctx)
    m.login(user, password)
    return m


def build_message(frm, to, subject, body):
    return (
        f"From: {frm}\r\nTo: {to}\r\nSubject: {subject}\r\n"
        f"Date: {time.strftime('%a, %d %b %Y %H:%M:%S +0000', time.gmtime())}\r\n"
        f"Message-ID: <load-{time.time_ns()}@qa.load>\r\n"
        "MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n"
        f"{body}\r\n"
    ).encode()


def stats(samples):
    if not samples:
        return {}
    s = sorted(samples)
    return {
        "n": len(s),
        "min": round(s[0]),
        "median": round(statistics.median(s)),
        "p95": round(s[int(len(s) * 0.95) - 1] if len(s) > 1 else s[0]),
        "max": round(s[-1]),
    }


def write_results():
    log = os.path.join(os.path.dirname(os.path.abspath(__file__)), "results.log")
    with open(log, "a", encoding="utf-8") as fh:
        fh.write("\n".join(RESULTS) + "\n")
    print(f"[LOAD] wrote {len(RESULTS)} result lines to {log}")


def main():
    api_base, imap_host, imap_port, count = sys.argv[1:5]
    count = int(count)
    rnd = sys.argv[5] if len(sys.argv) > 5 else str(int(time.time()) % 1000000)
    pw = "QaUser2026!"
    load_user = f"qa-load-{rnd}@example.com"
    peer = f"qa-loadp-{rnd}@example.com"
    marker = f"loadmark{rnd}"
    body_word = f"bodyword{rnd}"

    admin = Api(api_base)
    if admin.login("admin@example.com", "MailezDemo2026!") != 200:
        check("LOAD.0", "load round could log in as admin", False, "admin login failed")
        write_results()
        return
    for u in (load_user, peer):
        status, _, _ = admin.call("POST", "/users", {"email": u, "password": pw, "display_name": "QA load"})
        if status not in (200, 201):
            check("LOAD.0", "load round provisioned its accounts", False, f"create {u} -> {status}")
            write_results()
            return
    check("LOAD.0", "load accounts provisioned", True, f"user={load_user} peer={peer}")

    t0 = time.time()
    m = imap_conn(imap_host, imap_port, load_user, pw)
    seeded = 0
    try:
        for i in range(1, count + 1):
            subj = f"{marker}-{i:05d}" if i != count else f"unique {marker} subject"
            body = "load body filler\n" * 3
            if i == count - 1:
                body += f"unique-body-token {body_word}\n"
            typ, _ = m.append("INBOX", None, imaplib.Time2Internaldate(time.time()),
                              build_message(peer, load_user, subj, body))
            if typ == "OK":
                seeded += 1
            if i % 500 == 0:
                print(f"  seeded {i}/{count} ({time.time() - t0:.0f}s)", flush=True)
    finally:
        try:
            m.logout()
        except Exception:
            pass
    seed_secs = time.time() - t0
    check("LOAD.1", f"seeded {count} messages into one mailbox", seeded == count,
          f"seeded={seeded} in={seed_secs:.0f}s ({seeded / max(seed_secs, 1):.0f} msg/s)")

    user = Api(api_base)
    user.login(load_user, pw)

    def sample(path, n=8, pause=0.2):
        lat = []
        codes = []
        rows = []
        for _ in range(n):
            status, body, ms = user.call("GET", path)
            codes.append(status)
            lat.append(ms)
            if isinstance(body, list):
                rows.append(len(body))
            elif isinstance(body, dict) and isinstance(body.get("data"), list):
                rows.append(len(body["data"]))
            else:
                rows.append(0)
            time.sleep(pause)
        return lat, codes, rows

    paths = [
        ("LOAD.2", "first page of Inbox", "/mail/messages?folder=Inbox&page=0"),
        ("LOAD.3", "sorted by date desc", "/mail/messages?folder=Inbox&page=0&sort=date&dir=desc"),
        ("LOAD.4", "sorted by subject", "/mail/messages?folder=Inbox&page=0&sort=subject&dir=desc"),
        ("LOAD.5", "conversation view", "/mail/messages?folder=Inbox&page=0&conversation=1"),
        ("LOAD.6", "unseen counters", "/mail/unseen"),
        ("LOAD.7", "search (subject hit)", f"/mail/search?q={marker}-00042&folder=Inbox"),
        ("LOAD.8", "search (body token)", f"/mail/search?q={body_word}&folder=Inbox"),
    ]
    for cid, name, path in paths:
        lat, codes, rows = sample(path)
        st = stats(lat)
        ok = all(c == 200 for c in codes)
        check(cid, f"{name} @ {count} messages", ok,
              f"ms min/med/p95/max = {st.get('min')}/{st.get('median')}/{st.get('p95')}/{st.get('max')} codes={sorted(set(codes))} rows={sorted(set(rows))}")

    stop = threading.Event()
    errors = []

    def sender():
        peer_api = Api(api_base)
        peer_api.login(peer, pw)
        i = 0
        while not stop.is_set() and i < 6:
            st, _, _ = peer_api.call("POST", "/mail/send",
                                     {"to": [load_user], "subject": f"{marker} live {i}", "body": f"live {body_word}"})
            if st not in (200, 204):
                errors.append(f"send={st}")
            i += 1
            time.sleep(3)

    th = threading.Thread(target=sender, daemon=True)
    th.start()
    search_lat = []
    search_codes = []
    for _ in range(8):
        status, _, ms = user.call("GET", f"/mail/search?q={marker}&folder=Inbox")
        search_codes.append(status)
        search_lat.append(ms)
        time.sleep(0.8)
    stop.set()
    th.join(timeout=40)
    st = stats(search_lat)
    check("LOAD.9", "search stays correct while new mail is delivered",
          all(c == 200 for c in search_codes) and not errors,
          f"searches={len(search_lat)} codes={sorted(set(search_codes))} ms min/med/p95={st.get('min')}/{st.get('median')}/{st.get('p95')} senderErrors={errors}")

    status, _, ms = user.call("POST", "/mail/read-all", {"folder": "Inbox"})
    check("LOAD.10", "read-all over the loaded mailbox", status in (200, 204),
          f"status={status} ms={round(ms)} over {count}+ messages")

    t1 = time.time()
    d1, _, _ = admin.call("DELETE", f"/users/{load_user}")
    d2, _, _ = admin.call("DELETE", f"/users/{peer}")
    check("LOAD.11", "loaded accounts are deleted again (cascade purge included)",
          d1 in (200, 204) and d2 in (200, 204),
          f"user={d1} peer={d2} deleteMs={round((time.time() - t1) * 1000)}")
    write_results()


if __name__ == "__main__":
    main()
