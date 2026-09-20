#!/usr/bin/env python3
r"""10k-scale load round: seeds one mailbox with parallel IMAP APPENDs, then
samples the read paths (list / sorted / conversation / unseen / search) and
runs searches while new mail is arriving.

usage: qa-p6c-load10k.py <api_base> <imap_host> <imap_port> <count> <conns> [rnd]
Appends QA-format result lines to qa/results.log.
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

    def call(self, method, path, body=None, timeout=300):
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
        f"Message-ID: <load10k-{time.time_ns()}-{threading.get_ident()}@qa.load>\r\n"
        "MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n"
        f"{body}\r\n"
    ).encode()


def stats(samples):
    if not samples:
        return {}
    s = sorted(samples)
    return {"min": round(s[0]), "median": round(statistics.median(s)),
            "p95": round(s[int(len(s) * 0.95) - 1] if len(s) > 1 else s[0]), "max": round(s[-1])}


def write_results():
    log = os.path.join(os.path.dirname(os.path.abspath(__file__)), "results.log")
    with open(log, "a", encoding="utf-8") as fh:
        fh.write("\n".join(RESULTS) + "\n")
    print(f"[LOAD10K] wrote {len(RESULTS)} result lines to {log}")


def main():
    api_base, imap_host, imap_port, count, conns = sys.argv[1:6]
    count, conns = int(count), int(conns)
    rnd = sys.argv[6] if len(sys.argv) > 6 else str(int(time.time()) % 1000000)
    pw = "QaUser2026!"
    load_user = f"qa-load10k-{rnd}@example.com"
    peer = f"qa-load10kp-{rnd}@example.com"
    marker = f"k10mark{rnd}"
    body_word = f"k10body{rnd}"

    admin = Api(api_base)
    if admin.login("admin@example.com", "MailezDemo2026!") != 200:
        check("L10K.0", "10k round could log in as admin", False, "admin login failed")
        write_results()
        return
    for u in (load_user, peer):
        status, _, _ = admin.call("POST", "/users", {"email": u, "password": pw, "display_name": "QA load10k"})
        if status not in (200, 201):
            check("L10K.0", "10k round provisioned its accounts", False, f"create {u} -> {status}")
            write_results()
            return
    check("L10K.0", "10k accounts provisioned", True, f"user={load_user} peer={peer}")

    t0 = time.time()
    errors = []
    per_thread = count // conns
    lock = threading.Lock()
    done = {"n": 0}

    def seed(worker, n):
        try:
            m = imap_conn(imap_host, imap_port, load_user, pw)
        except Exception as exc:  # noqa: BLE001
            with lock:
                errors.append(f"worker{worker} connect: {exc}")
            return
        try:
            for i in range(n):
                idx = worker * per_thread + i
                subj = f"{marker}-{idx:06d}"
                body = "load body filler for the 10k round\n" * 2
                if idx == 1:
                    body += f"unique-body-token {body_word}\n"
                typ, _ = m.append("INBOX", None, imaplib.Time2Internaldate(time.time()),
                                  build_message(peer, load_user, subj, body))
                if typ != "OK":
                    with lock:
                        errors.append(f"append {idx} -> {typ}")
                with lock:
                    done["n"] += 1
                    if done["n"] % 1000 == 0:
                        print(f"  seeded {done['n']}/{count} ({time.time() - t0:.0f}s)", flush=True)
        finally:
            try:
                m.logout()
            except Exception:
                pass

    threads = [threading.Thread(target=seed, args=(w, per_thread)) for w in range(conns)]
    for th in threads:
        th.start()
    for th in threads:
        th.join()
    seed_secs = time.time() - t0
    check("L10K.1", f"seeded {count} messages with {conns} parallel APPEND sessions",
          done["n"] >= count and not errors,
          f"seeded={done['n']} in={seed_secs:.0f}s ({done['n'] / max(seed_secs, 1):.1f} msg/s) errors={errors[:3]}")

    user = Api(api_base)
    user.login(load_user, pw)

    def sample(path, n=8, pause=0.3):
        lat, codes, rows = [], [], []
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
        ("L10K.2", "first page of Inbox", "/mail/messages?folder=Inbox&page=0"),
        ("L10K.3", "sorted by date desc", "/mail/messages?folder=Inbox&page=0&sort=date&dir=desc"),
        ("L10K.4", "conversation view", "/mail/messages?folder=Inbox&page=0&conversation=1"),
        ("L10K.5", "unseen counters", "/mail/unseen"),
        ("L10K.6", "search (subject hit)", f"/mail/search?q={marker}-000042&folder=Inbox"),
        ("L10K.7", "search (body token)", f"/mail/search?q={body_word}&folder=Inbox"),
        ("L10K.8", "page 5 of the list", "/mail/messages?folder=Inbox&page=5"),
    ]
    for cid, name, path in paths:
        lat, codes, rows = sample(path)
        st = stats(lat)
        check(cid, f"{name} @ {count} messages", all(c == 200 for c in codes),
              f"ms min/med/p95/max = {st.get('min')}/{st.get('median')}/{st.get('p95')}/{st.get('max')} codes={sorted(set(codes))} rows={sorted(set(rows))}")

    stop = threading.Event()
    send_errors = []

    def sender():
        peer_api = Api(api_base)
        peer_api.login(peer, pw)
        i = 0
        while not stop.is_set() and i < 6:
            st, _, _ = peer_api.call("POST", "/mail/send",
                                     {"to": [load_user], "subject": f"{marker} live {i}", "body": f"live {body_word}"})
            if st not in (200, 204):
                send_errors.append(f"send={st}")
            i += 1
            time.sleep(3)

    th = threading.Thread(target=sender, daemon=True)
    th.start()
    search_lat, search_codes = [], []
    for _ in range(8):
        status, _, ms = user.call("GET", f"/mail/search?q={marker}&folder=Inbox")
        search_codes.append(status)
        search_lat.append(ms)
        time.sleep(0.8)
    stop.set()
    th.join(timeout=60)
    st = stats(search_lat)
    check("L10K.9", "search stays correct while new mail is delivered @ 10k",
          all(c == 200 for c in search_codes) and not send_errors,
          f"searches={len(search_lat)} codes={sorted(set(search_codes))} ms min/med/p95={st.get('min')}/{st.get('median')}/{st.get('p95')} senderErrors={send_errors}")

    t1 = time.time()
    d1, _, _ = admin.call("DELETE", f"/users/{load_user}")
    d2, _, _ = admin.call("DELETE", f"/users/{peer}")
    check("L10K.10", "10k mailbox is deleted again (cascade purge included)",
          d1 in (200, 204) and d2 in (200, 204), f"user={d1} peer={d2} deleteMs={round((time.time() - t1) * 1000)}")
    write_results()


if __name__ == "__main__":
    main()
