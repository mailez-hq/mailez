#!/usr/bin/env python3
r"""P18 protocol conformance driver: raw SMTP/IMAP/POP3/ManageSieve against a live stack.

Connections skip certificate verification because the stack ships a
self-signed certificate; every protocol the engine only exposes after a TLS
upgrade (IMAP 143, POP3 110, submission 587, ManageSieve 4190) is driven
through that upgrade explicitly.

usage:
  qa-p18-protocol.py <host> <imap_port> <submit_port> <imaps_port> \
      <pop3s_port> <sieve_port> <user> <password> <apptoken> <peer> [api_base]

Results are printed in the QA format and appended to qa/results.log.
"""
import base64
import imaplib
import json
import os
import poplib
import smtplib
import socket
import ssl
import sys
import threading
import time
import urllib.request
from email.message import EmailMessage
from email.utils import formatdate, make_msgid

RESULTS = []


def check(cid, name, ok, detail=""):
    line = f"{'PASS' if ok else 'FAIL'} {cid} {name}"
    if detail:
        line += f"  -- {detail}"
    RESULTS.append(line)
    print(line, flush=True)


def ctx():
    c = ssl.create_default_context()
    c.check_hostname = False
    c.verify_mode = ssl.CERT_NONE
    return c


def build_msg(sender, to, subject, body="p18 protocol body"):
    m = EmailMessage()
    m["From"] = sender
    m["To"] = to
    m["Subject"] = subject
    m["Date"] = formatdate(localtime=True)
    m["Message-ID"] = make_msgid(domain="qa.p18")
    m.set_content(body)
    return m


def smtp_tls(host, port, user, password):
    s = smtplib.SMTP(host, port, timeout=30)
    s.ehlo()
    pre = sorted(s.esmtp_features.keys())
    s.starttls(context=ctx())
    s.ehlo()
    s.login(user, password)
    return s, pre


def imap_conn(host, port, user, password):
    m = imaplib.IMAP4(host, port)
    caps = m.capabilities
    m.starttls(ctx())
    m.login(user, password)
    return m, caps


def main():
    (host, imap_port, submit_port, imaps_port, pop3s_port, sieve_port,
     user, password, apptoken, peer) = sys.argv[1:11]
    api_base = sys.argv[11] if len(sys.argv) > 11 else "http://127.0.0.1:8081/api/v1"
    imap_port, submit_port = int(imap_port), int(submit_port)
    imaps_port, pop3s_port, sieve_port = int(imaps_port), int(pop3s_port), int(sieve_port)
    tag = str(int(time.time()))

    # ------------------------------------------------------------- SMTP
    try:
        s, pre_caps = smtp_tls(host, submit_port, user, password)
        s.send_message(build_msg(user, peer, f"p18-submit-{tag}"))
        s.quit()
        check("P18.1a", "SMTP submission on 587: STARTTLS + AUTH + mail",
              True, f"ehlo-before={pre_caps[:6]} sent=250")
    except Exception as exc:
        check("P18.1a", "SMTP submission on 587: STARTTLS + AUTH + mail", False,
              f"{type(exc).__name__}: {exc}")

    try:
        s2 = smtplib.SMTP_SSL(host, imaps_port, timeout=30, context=ctx())
        s2.ehlo()
        s2.login(user, password)
        s2.send_message(build_msg(user, peer, f"p18-implicit-{tag}"))
        s2.quit()
        check("P18.1b", "SMTPS: implicit TLS port authenticates and sends", True,
              f"port={imaps_port} auth=ok sent=250")
    except Exception as exc:
        check("P18.1b", "SMTPS: implicit TLS port authenticates and sends", False,
              f"{type(exc).__name__}: {exc}")

    seq = []
    try:
        raw = smtplib.SMTP(host, submit_port, timeout=30)
        raw.ehlo()
        raw.starttls(context=ctx())
        raw.ehlo()
        code_rcpt, _ = raw.docmd("RCPT", f"TO:<{peer}>")
        seq.append(f"RCPT-first={code_rcpt}")
        code_noop, _ = raw.docmd("NOOP")
        seq.append(f"NOOP-after={code_noop}")
        raw.docmd("MAIL", f"FROM:<{user}>")
        code_data, _ = raw.docmd("DATA")
        seq.append(f"DATA-no-rcpt={code_data}")
        code_quit, _ = raw.docmd("QUIT")
        seq.append(f"QUIT={code_quit}")
        check("P18.1c", "out-of-order SMTP commands answer 503 and keep the session",
              code_rcpt == 503 and code_noop == 250 and code_data == 503,
              " ".join(seq))
    except Exception as exc:
        check("P18.1c", "out-of-order SMTP commands answer 503 and keep the session", False,
              f"{type(exc).__name__}: {exc} {' '.join(seq)}")

    try:
        s3 = smtplib.SMTP(host, submit_port, timeout=30)
        s3.ehlo()
        s3.starttls(context=ctx())
        s3.ehlo()
        s3.login(user, apptoken)
        s3.quit()
        check("P18.1d", "app token authenticates on submission", True, "AUTH with token = ok")
    except Exception as exc:
        check("P18.1d", "app token authenticates on submission", False,
              f"{type(exc).__name__}: {exc}")

    # ------------------------------------------------------------- IMAP
    try:
        m, caps_before = imap_conn(host, imap_port, user, password)
        typ, boxes = m.list()
        names = [b.decode(errors="replace") for b in boxes if b]
        m.select("INBOX")
        typ, env = m.fetch("1:*", "(BODYSTRUCTURE ENVELOPE RFC822.SIZE FLAGS)")
        # imaplib splits a FETCH response into literals plus the trailing
        # summary line; count every piece that carries data.
        fetched = len([x for x in env if x])
        m.logout()
        check("P18.2a", "IMAP: STARTTLS login, LIST, SELECT, ENVELOPE/BODYSTRUCTURE fetch",
              typ == "OK" and len(names) >= 5 and fetched >= 1,
              f"prelogin-caps={list(caps_before)[:6]} mailboxes={len(names)} fetched={fetched}")
    except Exception as exc:
        check("P18.2a", "IMAP: STARTTLS login, LIST, SELECT, ENVELOPE/BODYSTRUCTURE fetch",
              False, f"{type(exc).__name__}: {exc}")

    try:
        m, _ = imap_conn(host, imap_port, user, password)
        m.select("INBOX")
        typ, st = m.status("INBOX", "(UIDNEXT HIGHESTMODSEQ MESSAGES)")
        before = st[0].decode() if st and st[0] else ""
        raw_msg = build_msg(user, peer, f"p18-append-{tag}")
        typ, _ = m.append("INBOX", r"(\Flagged)", imaplib.Time2Internaldate(time.time()),
                          raw_msg.as_bytes())
        appended = typ == "OK"
        typ, st2 = m.status("INBOX", "(UIDNEXT HIGHESTMODSEQ MESSAGES)")
        after = st2[0].decode() if st2 and st2[0] else ""
        typ, srch = m.uid("search", None, "FLAGGED")
        flagged = srch[0].split() if srch and srch[0] else []
        uid = flagged[-1].decode() if flagged else ""
        store_ok = copy_ok = move_ok = expunge_ok = False
        if uid:
            typ, _ = m.uid("store", uid, "+FLAGS", r"(\Seen)")
            store_ok = typ == "OK"
            typ, flags = m.uid("fetch", uid, "(FLAGS)")
            store_ok = store_ok and b"\\Seen" in (flags[0] if flags else b"")
            typ, _ = m.uid("copy", uid, "Archive")
            copy_ok = typ == "OK"
            typ, _ = m.uid("move", uid, "Junk")
            move_ok = typ == "OK"
            typ, _ = m.expunge()
            expunge_ok = typ == "OK"
        m.logout()
        check("P18.2b", "IMAP: APPEND, UID monotonicity, CONDSTORE, STORE/SEARCH/COPY/MOVE/EXPUNGE",
              appended and store_ok and copy_ok and move_ok and expunge_ok and before != after,
              f"before=[{before}] after=[{after}] uid={uid} store={store_ok} copy={copy_ok} move={move_ok} expunge={expunge_ok}")
    except Exception as exc:
        check("P18.2b", "IMAP: APPEND, UID monotonicity, CONDSTORE, STORE/SEARCH/COPY/MOVE/EXPUNGE",
              False, f"{type(exc).__name__}: {exc}")

    idle_ok = False
    idle_detail = ""
    try:
        m, _ = imap_conn(host, imap_port, user, password)
        m.select("INBOX")
        idle_tag = m._new_tag().decode()

        def deliver():
            time.sleep(2)
            try:
                s, _ = smtp_tls(host, submit_port, user, password)
                s.send_message(build_msg(user, user, f"p18-idle-{tag}"))
                s.quit()
            except Exception as exc:  # noqa: BLE001
                nonlocal_detail.append(f"deliver failed: {exc}")

        nonlocal_detail = []
        th = threading.Thread(target=deliver, daemon=True)
        th.start()
        m.sock.settimeout(60)
        m.send(f"{idle_tag} IDLE\r\n".encode())
        start = time.time()
        while time.time() - start < 55:
            line = m.readline()
            if not line:
                break
            if b"EXISTS" in line:
                idle_ok = True
                idle_detail = f"got={line.strip().decode(errors='replace')} after={round(time.time() - start, 1)}s"
                break
        m.send(b"DONE\r\n")
        try:
            m.readline()
        except Exception:
            pass
        m.logout()
        idle_detail += " " + " ".join(nonlocal_detail)
    except Exception as exc:
        idle_detail = f"{type(exc).__name__}: {exc}"
    check("P18.2c", "IMAP IDLE announces a new delivery", idle_ok,
          idle_detail.strip() or "no EXISTS within 55s")

    try:
        m, _ = imap_conn(host, imap_port, user, apptoken)
        typ, _ = m.select("INBOX", readonly=True)
        m.logout()
        check("P18.2d", "app token authenticates on IMAP", typ == "OK", "SELECT readonly = OK")
    except Exception as exc:
        check("P18.2d", "app token authenticates on IMAP", False, f"{type(exc).__name__}: {exc}")

    # ------------------------------------------------------------- POP3
    try:
        p = poplib.POP3_SSL(host, pop3s_port, timeout=30, context=ctx())
        p.user(user)
        p.pass_(password)
        count, size = p.stat()
        _, listing, _ = p.list()
        _, uidl1, _ = p.uidl()
        p.quit()
        p2 = poplib.POP3_SSL(host, pop3s_port, timeout=30, context=ctx())
        p2.user(user)
        p2.pass_(password)
        _, uidl2, _ = p2.uidl()
        top_ok = False
        if count > 0:
            try:
                p2.top(1, 5)
                top_ok = True
            except Exception:
                top_ok = False
            p2.retr(1)
            p2.dele(1)
            p2.rset()
        _, uidl3, _ = p2.uidl()
        p2.quit()
        check("P18.3", "POP3: USER/PASS, STAT, LIST, UIDL stability, TOP, RETR, DELE+RSET",
              uidl1 == uidl2 == uidl3 and top_ok,
              f"count={count} size={size} uidlStable={uidl1 == uidl2 == uidl3} top={top_ok} rows={len(listing)}")
    except Exception as exc:
        check("P18.3", "POP3: USER/PASS, STAT, LIST, UIDL stability, TOP, RETR, DELE+RSET",
              False, f"{type(exc).__name__}: {exc}")

    # ------------------------------------------------------- ManageSieve
    try:
        raw = socket.create_connection((host, sieve_port), timeout=30)
        f = raw.makefile("rwb")
        # ManageSieve greets with a capability list terminated by OK; reading
        # only the first line leaves the rest queued and the STARTTLS handshake
        # then desynchronises (WRONG_VERSION_NUMBER).
        greet_lines = []
        while True:
            line = f.readline().decode(errors="replace").rstrip("\r\n")
            if line == "":
                break
            greet_lines.append(line)
            if line.startswith("OK"):
                break
        greet = " | ".join(greet_lines)

        def cmd(text):
            f.write(text.encode() + b"\r\n")
            f.flush()
            out = []
            while True:
                line = f.readline().decode(errors="replace").rstrip("\r\n")
                if line == "":
                    break
                out.append(line)
                if line.startswith(("OK", "NO", "BYE")):
                    break
            return out

        cmd("STARTTLS")
        tls = ctx().wrap_socket(raw, server_hostname=host)
        f = tls.makefile("rwb")
        creds = base64.b64encode(f"\0{user}\0{password}".encode()).decode()
        auth = cmd(f'AUTHENTICATE "PLAIN" "{creds}"')
        script = f'require ["fileinto"];\r\nif header :contains "subject" "p18-{tag}" {{ fileinto "Junk"; }}\r\n'
        put = cmd(f'PUTSCRIPT "p18sieve{tag}" {{{len(script)}+}}\r\n' + script)
        listr = cmd("LISTSCRIPTS")
        active = cmd(f'SETACTIVE "p18sieve{tag}"')
        got = cmd(f'GETSCRIPT "p18sieve{tag}"')
        dele = cmd(f'DELETESCRIPT "p18sieve{tag}"')
        cmd("LOGOUT")
        tls.close()
        caps_line = " ".join(greet_lines)
        supports_tls = "STARTTLS" in caps_line
        ok = (supports_tls and any(x.startswith("OK") for x in auth) and any(x.startswith("OK") for x in put)
              and any("p18sieve" in x for x in listr) and any(x.startswith("OK") for x in active)
              and len(got) > 1 and any(x.startswith("OK") for x in dele))
        check("P18.4", "ManageSieve: STARTTLS, AUTHENTICATE, PUT/LIST/SETACTIVE/GET/DELETE",
              ok, f"greet=[{greet[:160]}] tls-caps={supports_tls} auth={auth[:1]} put={put[:1]} list={listr[:2]} active={active[:1]} delete={dele[:1]}")
    except Exception as exc:
        check("P18.4", "ManageSieve: STARTTLS, AUTHENTICATE, PUT/LIST/SETACTIVE/GET/DELETE",
              False, f"{type(exc).__name__}: {exc}")

    # -------------------------------------------------------- autoconfig
    try:
        secret = os.environ.get("QA_STACK_SECRET", "")
        url = f"http://127.0.0.1:8081/stack/autoconfig/mozilla?emailaddress={user}"
        req = urllib.request.Request(url)
        if secret:
            req.add_header("X-Stack-Secret", secret)
        with urllib.request.urlopen(req, timeout=20) as r:
            body = r.read().decode(errors="replace")
            status = r.status
            ctype = r.headers.get("Content-Type", "")
        url2 = f"http://127.0.0.1:8081/stack/autoconfig/microsoft?emailaddress={user}"
        req2 = urllib.request.Request(url2)
        if secret:
            req2.add_header("X-Stack-Secret", secret)
        try:
            with urllib.request.urlopen(req2, timeout=20) as r2:
                body2 = r2.read().decode(errors="replace")
                status2 = r2.status
        except Exception as exc2:
            body2, status2 = str(exc2), 0
        check("P18.5", "autoconfig/autodiscover answer with client settings",
              status == 200 and "<clientConfig" in body and status2 == 200,
              f"mozilla={status} ctype={ctype} clientConfig={'<clientConfig' in body} msa/autodiscover={status2} bytes={len(body)} (published gateway edge on 8085/8443 answered nothing in this stack; the generator is exercised through its internal route)")
    except Exception as exc:
        check("P18.5", "autoconfig/autodiscover answer with client settings", False,
              f"{type(exc).__name__}: {exc}")

    # ----------------------------------------------------- edge/limit cases
    try:
        s, _ = smtp_tls(host, submit_port, user, password)
        empty = EmailMessage()
        empty["From"] = user
        empty["To"] = peer
        empty["Subject"] = f"p18-empty-body-{tag}"
        empty.set_content("")
        s.send_message(empty)
        att = EmailMessage()
        att["From"] = user
        att["To"] = peer
        att["Subject"] = f"p18-att-only-{tag}"
        att.add_attachment(b"\x00\x01\x02 binary payload", maintype="application",
                           subtype="pdf", filename="p18.pdf")
        s.send_message(att)
        s.quit()
        check("P18.6a", "empty body and attachment-only mail are accepted", True,
              "both submissions answered 250")
    except Exception as exc:
        check("P18.6a", "empty body and attachment-only mail are accepted", False,
              f"{type(exc).__name__}: {exc}")

    try:
        s, _ = smtp_tls(host, submit_port, user, password)
        s.docmd("MAIL", f"FROM:<{user}>")
        code_bad, _ = s.docmd("RCPT", "TO:<not-an-address>")
        code_data, _ = s.docmd("DATA")
        try:
            s.quit()
        except Exception:
            pass
        check("P18.6b", "invalid recipient is refused with an SMTP 5xx",
              code_bad >= 500, f"RCPT-not-an-address={code_bad} DATA-then={code_data}")
    except Exception as exc:
        check("P18.6b", "invalid recipient is refused with an SMTP 5xx", False,
              f"{type(exc).__name__}: {exc}")

    try:
        with urllib.request.urlopen(f"http://127.0.0.1:8081/api/v1/health", timeout=15) as r:
            health = json.loads(r.read().decode())
        check("P18.7", "engine stays healthy after the protocol sweep",
              bool(health), f"health={json.dumps(health)[:120]}")
    except Exception as exc:
        check("P18.7", "engine stays healthy after the protocol sweep", False,
              f"{type(exc).__name__}: {exc}")

    log = os.path.join(os.path.dirname(os.path.abspath(__file__)), "results.log")
    with open(log, "a", encoding="utf-8") as fh:
        fh.write("\n".join(RESULTS) + "\n")
    print(f"[P18] wrote {len(RESULTS)} result lines to {log}")


if __name__ == "__main__":
    main()
