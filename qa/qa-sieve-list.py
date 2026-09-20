#!/usr/bin/env python3
r"""Minimal ManageSieve client for QA: LISTSCRIPTS (+ optional GETSCRIPT).

The ManageSieve greeting is a multi-line capability list terminated by OK.
Reading only the first line leaves the rest queued, which desynchronises the
STARTTLS/DONE dance — the first P12/P18 runs both tripped over this and mis-read
"active script" state.

usage:
  qa-sieve-list.py <host> <port> <user> <password> [script-to-fetch]
prints one JSON object: {ok, scripts:[{name, active}], content, raw}
"""
import base64
import json
import socket
import ssl
import sys


def read_reply(f):
    lines = []
    while True:
        line = f.readline().decode(errors="replace").rstrip("\r\n")
        if line == "":
            break
        lines.append(line)
        if line.startswith(("OK", "NO", "BYE")):
            break
    return lines


def main():
    host, port, user, password = sys.argv[1:5]
    fetch = sys.argv[5] if len(sys.argv) > 5 else ""
    out = {"ok": False, "scripts": [], "content": "", "raw": []}
    try:
        raw = socket.create_connection((host, int(port)), timeout=30)
        f = raw.makefile("rwb")
        greet = read_reply(f)
        out["raw"].extend(greet)
        out["greet"] = " | ".join(greet)
        f.write(b"STARTTLS\r\n")
        f.flush()
        out["raw"].extend(read_reply(f))
        ctx = ssl.create_default_context()
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE
        tls = ctx.wrap_socket(raw, server_hostname=host)
        f = tls.makefile("rwb")
        creds = base64.b64encode(f"\0{user}\0{password}".encode()).decode()
        f.write(f'AUTHENTICATE "PLAIN" "{creds}"\r\n'.encode())
        f.flush()
        auth = read_reply(f)
        out["raw"].extend(auth)
        out["auth"] = auth
        if not any(x.startswith("OK") for x in auth):
            print(json.dumps(out))
            return
        f.write(b"LISTSCRIPTS\r\n")
        f.flush()
        listing = read_reply(f)
        for line in listing:
            if line.startswith(("OK", "NO", "BYE")):
                break
            name, _, flags = line.partition(" ")
            out["scripts"].append({
                "name": name.strip('"'),
                "active": "ACTIVE" in flags.upper(),
            })
        out["raw"].extend(listing)
        if fetch:
            f.write(f'GETSCRIPT "{fetch}"\r\n'.encode())
            f.flush()
            body = read_reply(f)
            out["content"] = "\n".join(body[:-1]) if len(body) > 1 else ""
            out["raw"].extend(body)
        f.write(b"LOGOUT\r\n")
        f.flush()
        tls.close()
        out["ok"] = True
    except Exception as exc:
        out["error"] = f"{type(exc).__name__}: {exc}"
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    main()
