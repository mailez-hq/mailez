#!/usr/bin/env python3
r"""Minimal IMAP driver for mailez QA (P14 multi-client checks, P18 protocol run).

Talks to the engine over STARTTLS on the plaintext port (the engine advertises
LOGINDISABLED until the upgrade, so this is the only way in on 143) and skips
certificate verification because the stack ships a self-signed cert.

usage:
  python qa-imap-op.py <host> <port> <user> <password> <op> [arg]
ops:
  seen                 mark the newest INBOX message \Seen, print its UID
  unread               clear \Seen on the newest INBOX message
  seenuid <uid>        mark a specific INBOX UID \Seen (no SEARCH — the LB
                       round-robins between engine nodes and SEARCH results
                       can flap between them)
  moveuid <uid> <box>  move a specific INBOX UID to <box>
  move <folder>        move the newest INBOX message to <folder>
  list                 list mailboxes
  listuid <folder>     print UIDs in a folder
  subjects <folder>    print (uid, subject) pairs
"""
import imaplib
import json
import ssl
import sys


def connect(host, port, user, password):
    ctx = ssl._create_unverified_context()
    m = imaplib.IMAP4(host, int(port))
    if "STARTTLS" in m.capabilities:
        m.starttls(ctx)
    m.login(user, password)
    return m


def newest_uid(m, folder="INBOX"):
    m.select(folder, readonly=False)
    typ, data = m.uid("search", None, "ALL")
    uids = data[0].split()
    return uids[-1].decode() if uids else ""


def main():
    host, port, user, password, op = sys.argv[1:6]
    arg = sys.argv[6] if len(sys.argv) > 6 else ""
    out = {"op": op, "host": host, "port": int(port)}
    try:
        m = connect(host, port, user, password)
        out["login"] = "ok"
        if op == "list":
            typ, boxes = m.list()
            out["mailboxes"] = [
                b.decode(errors="replace") for b in boxes if b is not None
            ]
        elif op in ("seen", "unread"):
            uid = newest_uid(m)
            if uid:
                flag = "+FLAGS" if op == "seen" else "-FLAGS"
                out["status"], out["resp"] = m.uid("store", uid, flag, r"(\Seen)")
            out["uid"] = uid
        elif op == "seenuid":
            m.select("INBOX", readonly=False)
            out["uid"] = arg
            out["status"], out["resp"] = m.uid("store", arg, "+FLAGS", r"(\Seen)")
        elif op == "moveuid":
            # second argument carries the destination folder
            dest = sys.argv[7] if len(sys.argv) > 7 else "Junk"
            m.select("INBOX", readonly=False)
            out["uid"] = arg
            out["folder"] = dest
            out["status"], out["resp"] = m.uid("move", arg, dest)
        elif op == "move":
            uid = newest_uid(m)
            if uid:
                out["status"], out["resp"] = m.uid("move", uid, arg)
            out["uid"] = uid
            out["folder"] = arg
        elif op == "listuid":
            m.select(arg or "INBOX")
            typ, data = m.uid("search", None, "ALL")
            out["uids"] = [u.decode() for u in data[0].split()]
        elif op == "subjects":
            m.select(arg or "INBOX")
            typ, data = m.uid("search", None, "ALL")
            pairs = []
            for u in data[0].split():
                t, d = m.uid("fetch", u, "(BODY.PEEK[HEADER.FIELDS (SUBJECT)])")
                subj = ""
                for part in d:
                    if isinstance(part, tuple):
                        subj = part[1].decode(errors="replace").strip()
                pairs.append([u.decode(), subj])
            out["subjects"] = pairs
        else:
            out["error"] = "unknown op"
        try:
            m.logout()
        except Exception:
            pass
    except Exception as exc:  # noqa: BLE001 - QA driver reports the raw error
        out["error"] = f"{type(exc).__name__}: {exc}"
    print(json.dumps(out, ensure_ascii=False, default=str))


if __name__ == "__main__":
    main()
