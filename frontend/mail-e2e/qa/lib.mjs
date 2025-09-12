// Shared harness for the mailez usability audit (2026-09-11).
//
// Design notes:
// - Login is cached per process (backend rate-limits /sso/login) — reuse
//   ./helper.mjs's apiLogin so the whole suite keeps one session.
// - Every mutation is seeded with an "E2E-" subject and cleaned up in a
//   finally block; nothing without that prefix is ever touched.
// - "Second opinion" checks go through a dependency-free line-based IMAP
//   client (plaintext :143). Only commands whose responses are line-based are
//   used: any literal ({n}) would need byte-level framing and is rejected
//   loudly instead of mis-parsed.
import net from "node:net";
import tls from "node:tls";
import { mkdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

import { apiLogin, USER } from "../helper.mjs";

export { USER };
export const BASE = "http://127.0.0.1:8083";
export const ADMIN_BASE = "http://127.0.0.1:18082";
// The engine refuses plaintext authentication ([PRIVACYREQUIRED] TLS is
// required to authenticate), so protocol checks use the implicit-TLS port.
// The development stack ships a self-signed cert.
export const IMAP = { host: "127.0.0.1", port: 993, tls: true };

const here = dirname(fileURLToPath(import.meta.url));
export const ARTIFACTS = resolve(here, "..", "artifacts");
export const REPORTS = resolve(here, "..", "reports");
mkdirSync(ARTIFACTS, { recursive: true });
mkdirSync(REPORTS, { recursive: true });

// ---------------------------------------------------------------- results
const results = [];
export function record(module, name, status, detail = "") {
  results.push({ module, name, status, detail });
  console.log(`${status.padEnd(7)} [${module}] ${name}${detail ? ` — ${detail}` : ""}`);
  return status === "PASS";
}
export function check(module, name, ok, detail = "") {
  return record(module, name, ok ? "PASS" : "FAIL", detail);
}
export function blocked(module, name, detail) {
  return record(module, name, "BLOCKED", detail);
}
export function resultsSoFar() {
  return results.slice();
}
export function summary() {
  const n = (s) => results.filter((r) => r.status === s).length;
  return { total: results.length, pass: n("PASS"), fail: n("FAIL"), blocked: n("BLOCKED"), results: results.slice() };
}
export function dumpResults(file) {
  const path = join(REPORTS, file);
  const payload = { generatedAt: new Date().toISOString(), ...summary() };
  // fs is only needed here; keep the import list tight at the top.
  return import("node:fs").then(({ writeFileSync }) => {
    writeFileSync(path, JSON.stringify(payload, null, 2));
    console.log(`results -> ${path}`);
    return payload;
  });
}

// ------------------------------------------------------------------- api
let cookieCache = null;
export async function cookie() {
  if (!cookieCache) {
    const c = await apiLogin();
    cookieCache = `${c.name}=${c.value}`;
  }
  return cookieCache;
}

export async function api(path, { method = "GET", body, base = BASE, headers = {}, raw = false } = {}) {
  const res = await fetch(`${base}${path}`, {
    method,
    headers: { "Content-Type": "application/json", cookie: await cookie(), ...headers },
    body: body === undefined ? undefined : raw ? body : JSON.stringify(body),
  });
  const text = await res.text();
  let json = null;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    /* non-JSON body (204 / "OK") */
  }
  return { status: res.status, json, text, total: Number(res.headers.get("X-Total-Messages") || 0) };
}

export async function listFolder(folder = "Inbox", page = 0, opts = {}) {
  const { sort = "date", dir = "desc", conversation = false } = opts;
  const r = await api(
    `/api/v1/mail/messages?folder=${encodeURIComponent(folder)}&page=${page}&sort=${sort}&dir=${dir}` +
      (conversation ? "&conversation=1" : ""),
  );
  return { status: r.status, messages: Array.isArray(r.json) ? r.json : [], total: r.total };
}

export async function seedMail(subject, opts = {}) {
  const {
    to = [USER.email],
    cc = [],
    bcc = [],
    body = "seed body",
    html,
    attachments,
    undo_seconds = 0,
    send_at,
    receipt_requested = false,
    burn_after_minutes = 0,
    in_reply_to,
    references,
  } = opts;
  const r = await api("/api/v1/mail/send", {
    method: "POST",
    body: {
      to, cc, bcc, subject, body,
      ...(html !== undefined ? { html } : {}),
      ...(attachments !== undefined ? { attachments } : {}),
      undo_seconds,
      ...(send_at ? { send_at } : {}),
      receipt_requested,
      burn_after_minutes,
      ...(in_reply_to ? { in_reply_to } : {}),
      ...(references ? { references } : {}),
    },
  });
  if (r.status !== 200 && r.status !== 202 && r.status !== 204) {
    throw new Error(`seedMail(${subject}) failed: ${r.status} ${r.text.slice(0, 200)}`);
  }
  return r;
}

/** Poll the API until a message whose subject contains `subject` shows up. */
export async function awaitMail(subject, { folder = "Inbox", timeoutMs = 60000 } = {}) {
  const start = Date.now();
  for (;;) {
    const { messages } = await listFolder(folder, 0);
    const hit = messages.find((m) => (m.subject || "").includes(subject));
    if (hit) return hit;
    if (Date.now() - start > timeoutMs) return null;
    await sleep(1500);
  }
}

export async function waitGone(subject, { folder = "Inbox", timeoutMs = 30000 } = {}) {
  const start = Date.now();
  for (;;) {
    const { messages } = await listFolder(folder, 0);
    if (!messages.some((m) => (m.subject || "").includes(subject))) return true;
    if (Date.now() - start > timeoutMs) return false;
    await sleep(1500);
  }
}

/** Cleanup: move every E2E-* row of a folder to Trash (never touches demo mail). */
export async function cleanupFolder(folder = "Inbox") {
  for (let page = 0; page < 8; page++) {
    const { messages } = await listFolder(folder, page);
    const uids = messages.filter((m) => /^E2E-/.test(m.subject || "")).map((m) => m.uid);
    if (!uids.length) return;
    await api("/api/v1/mail/move", { method: "POST", body: { folder, uids, destination: "Trash" } });
    await sleep(600);
  }
}

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

/**
 * Targeted purge of the exact subjects a scenario created: search each folder
 * over IMAP, flag \Deleted and expunge. Only E2E-* subjects are ever passed in,
 * and only those subjects are touched — a blanket expunge would risk other
 * people's in-flight test data.
 */
export async function purgeSubjects(
  subjects,
  folders = ["Inbox", "Drafts", "Sent", "Junk", "Trash", "Archive"],
  timeoutMs = 120000,
) {
  const wanted = subjects.filter((s) => /^E2E-/.test(s));
  if (!wanted.length) return 0;
  // One SEARCH per folder instead of one per subject: every subject of a run
  // shares its numeric stamp, and a mailbox SEARCH walks the whole mailbox, so
  // N searches in a 500-message Trash costs N × ~45s. Searching the shared
  // stamp matches exactly the same rows.
  const stamps = [...new Set(wanted.map((s) => (s.match(/(\d{10,})$/) || [s])[0]))];
  let purged = 0;
  await withImap(async (c) => {
    for (const folder of folders) {
      let uids = [];
      try {
        await c.select(folder, timeoutMs);
        for (const pattern of stamps) {
          const r = await c.cmd(`UID SEARCH HEADER Subject ${JSON.stringify(pattern)}`, timeoutMs);
          const line = r.lines.find((l) => /^\* SEARCH/.test(l)) || "";
          uids.push(...line.replace(/^\* SEARCH\s*/, "").trim().split(/\s+/).filter(Boolean).map(Number));
        }
      } catch (e) {
        // A missing folder, or a mailbox too large to answer inside the
        // command budget (see the report's performance notes).
        console.log(`  (purge: ${folder} skipped — ${e.message})`);
        continue;
      }
      if (!uids.length) continue;
      const set = [...new Set(uids)].join(",");
      await c.cmd(`UID STORE ${set} +FLAGS (\\Deleted)`, timeoutMs).catch(() => {});
      await c.cmd("EXPUNGE", timeoutMs).catch(() => {});
      purged += new Set(uids).size;
    }
  });
  return purged;
}

// ------------------------------------------------------------------ imap
const imapQuote = (s) => `"${String(s).replace(/\\/g, "\\\\").replace(/"/g, '\\"')}"`;

export class ImapClient {
  constructor({ host = IMAP.host, port = IMAP.port, tls: useTls = IMAP.tls } = {}) {
    this.host = host;
    this.port = port;
    this.useTls = useTls;
    this.lines = [];
    this.waiters = [];
    this.tag = 0;
    this.transcript = [];
  }

  static async connect(opts = {}) {
    const c = new ImapClient(opts);
    await c.#open();
    return c;
  }

  #open() {
    return new Promise((resolvePromise, rejectPromise) => {
      this.sock = this.useTls
        ? tls.connect({ host: this.host, port: this.port, rejectUnauthorized: false })
        : net.createConnection({ host: this.host, port: this.port });
      this.sock.setEncoding("utf8");
      let buf = "";
      const timer = setTimeout(() => rejectPromise(new Error("imap connect timeout")), 15000);
      const onData = (chunk) => {
        buf += chunk;
        let i;
        while ((i = buf.indexOf("\r\n")) >= 0) {
          const line = buf.slice(0, i);
          buf = buf.slice(i + 2);
          this.transcript.push(`< ${line}`);
          const w = this.waiters.shift();
          if (w) w(line);
          else this.lines.push(line);
        }
      };
      const onError = (e) => rejectPromise(e);
      this.sock.on("data", onData);
      this.sock.once("error", onError);
      // The first line the server sends is the greeting: hand it to the
      // connect promise instead of the command queue.
      this.waiters.push((line) => {
        clearTimeout(timer);
        this.sock.off("error", onError);
        resolvePromise(line);
      });
    });
  }

  #next(timeoutMs = 20000) {
    if (this.lines.length) return Promise.resolve(this.lines.shift());
    return new Promise((res, rej) => {
      const t = setTimeout(() => rej(new Error("imap read timeout")), timeoutMs);
      this.waiters.push((line) => {
        clearTimeout(t);
        res(line);
      });
    });
  }

  /** Send a tagged command, collect its response lines. */
  async cmd(command, timeoutMs = 45000) {
    const tag = `q${++this.tag}`;
    this.transcript.push(`> ${tag} ${command}`);
    this.sock.write(`${tag} ${command}\r\n`);
    const out = [];
    for (;;) {
      const line = await this.#next(timeoutMs);
      if (/\{\d+\}$/.test(line)) {
        throw new Error(`imap: literal response unsupported (${line.slice(0, 60)})`);
      }
      out.push(line);
      if (line.startsWith(`${tag} `)) {
        const ok = new RegExp(`^${tag} OK`, "i").test(line);
        return { ok, lines: out, status: line };
      }
    }
  }

  /** Read the next raw response line (APPEND's literal handshake needs it). */
  nextLine(timeoutMs = 45000) {
    return this.#next(timeoutMs);
  }

  async login(user = USER.email, pass = USER.pw) {
    const r = await this.cmd(`LOGIN ${imapQuote(user)} ${imapQuote(pass)}`);
    if (!r.ok) throw new Error(`imap login failed: ${r.status}`);
    return r;
  }

  async listMailboxes() {
    const r = await this.cmd('LIST "" "*"');
    const names = [];
    for (const line of r.lines) {
      const m = /^\* LIST \([^)]*\)\s+"[^"]*"\s+"?([^"]+)"?$/.exec(line.trim());
      if (m) names.push(m[1].trim());
    }
    return names;
  }

  async select(mailbox = "Inbox", timeoutMs = 45000) {
    const r = await this.cmd(`SELECT ${imapQuote(mailbox)}`, timeoutMs);
    const exists = /^\* (\d+) EXISTS/.exec(r.lines.find((l) => /EXISTS/.test(l)) || "");
    return { ok: r.ok, exists: exists ? Number(exists[1]) : null, lines: r.lines };
  }

  async status(mailbox = "Inbox") {
    const r = await this.cmd(`STATUS ${imapQuote(mailbox)} (MESSAGES UNSEEN UIDNEXT)`);
    const line = r.lines.find((l) => /MESSAGES/.test(l)) || "";
    const num = (k) => {
      const m = new RegExp(`${k} (\\d+)`).exec(line);
      return m ? Number(m[1]) : null;
    };
    return { ok: r.ok, messages: num("MESSAGES"), unseen: num("UNSEEN"), raw: line };
  }

  /** UID SEARCH with a raw criteria string, e.g. 'HEADER Subject "E2E-x"'. */
  async uidSearch(criteria, mailbox = "Inbox", timeoutMs = 45000) {
    await this.select(mailbox, timeoutMs).catch(() => {});
    const r = await this.cmd(`UID SEARCH ${criteria}`, timeoutMs);
    const line = r.lines.find((l) => /^\* SEARCH/.test(l)) || "* SEARCH";
    return line.replace(/^\* SEARCH\s*/, "").trim().split(/\s+/).filter(Boolean).map(Number);
  }

  /** Flags + size for one UID (line-based FETCH only). */
  async uidFetch(uid, mailbox = "Inbox") {
    await this.select(mailbox);
    const r = await this.cmd(`UID FETCH ${uid} (FLAGS RFC822.SIZE UID)`);
    const line = r.lines.find((l) => /\bFETCH\b/.test(l) && l.includes("UID")) || "";
    const flags = /FLAGS \(([^)]*)\)/.exec(line);
    const size = /RFC822\.SIZE (\d+)/.exec(line);
    return {
      ok: r.ok,
      found: /\bFETCH\b/.test(line),
      flags: flags ? flags[1].split(/\s+/).filter(Boolean) : [],
      size: size ? Number(size[1]) : null,
      raw: line,
    };
  }

  /** Copy to another mailbox then expunge the source copy (used for cleanup). */
  async uidMove(uid, destination, mailbox = "Inbox") {
    await this.select(mailbox);
    const copy = await this.cmd(`UID COPY ${uid} ${imapQuote(destination)}`).catch((e) => ({ ok: false, status: String(e) }));
    if (!copy.ok) return copy;
    await this.cmd(`UID STORE ${uid} +FLAGS (\\Deleted)`).catch(() => {});
    await this.cmd("EXPUNGE").catch(() => {});
    return copy;
  }

  async logout() {
    try {
      await this.cmd("LOGOUT", 5000);
    } catch {
      /* server may drop the socket */
    }
    this.sock.destroy();
  }
}

export async function withImap(fn, opts = {}) {
  const c = await ImapClient.connect(opts);
  try {
    await c.login();
    return await fn(c);
  } finally {
    await c.logout().catch(() => {});
  }
}

// ------------------------------------------------------------ browser/ui
let browserSingleton = null;
async function browser() {
  if (!browserSingleton) browserSingleton = await chromium.launch();
  return browserSingleton;
}
export async function closeBrowser() {
  if (browserSingleton) {
    await browserSingleton.close();
    browserSingleton = null;
  }
}

/**
 * New isolated browser context with the session cookie installed.
 * locale: pin NEXT_LOCALE so text assertions are deterministic.
 */
export async function newContext({ locale = "en", viewport = { width: 1440, height: 900 }, auth = true } = {}) {
  const b = await browser();
  const ctx = await b.newContext({ viewport });
  const cookies = [{ name: "NEXT_LOCALE", value: locale, domain: "127.0.0.1", path: "/" }];
  if (auth) {
    const c = await cookie();
    const [name, value] = [c.slice(0, c.indexOf("=")), c.slice(c.indexOf("=") + 1)];
    cookies.push({ name, value, domain: "127.0.0.1", path: "/" });
  }
  await ctx.addCookies(cookies);
  const page = await ctx.newPage();
  const logs = [];
  page.on("console", (m) => {
    if (m.type() === "error") logs.push(`CONSOLE ${m.text().slice(0, 300)}`);
  });
  page.on("pageerror", (e) => logs.push(`PAGEERROR ${String(e).slice(0, 300)}`));
  const failed = [];
  page.on("response", (r) => {
    if (r.status() >= 400) failed.push(`${r.status()} ${r.url().replace(BASE, "").slice(0, 80)}`);
  });
  return { ctx, page, logs, failed };
}

export async function gotoMail(page, path, { settleMs = 3000 } = {}) {
  await page.goto(`${BASE}${path}`, { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(settleMs);
}

export const rowLoc = (page, subject) =>
  page.locator(".mail-scroll [role='button']", { hasText: subject }).first();

export const listText = (page) => page.locator(".mail-scroll").last().innerText();

/** Row count stable across two samples (engine has a short stale window). */
export async function stableList(page, capMs = 45000) {
  const count = () => page.locator(".mail-scroll [role='button']").count();
  const start = Date.now();
  let a = await count();
  for (;;) {
    await page.waitForTimeout(1500);
    const b = await count();
    if (a === b && Date.now() - start > 2500) return b;
    a = b;
    if (Date.now() - start > capMs) return a;
  }
}

export async function shot(page, name) {
  const file = join(ARTIFACTS, `${name}.png`);
  await page.screenshot({ path: file });
  return `artifacts/${name}.png`;
}

/** Poll the on-screen list until `subject` appears (SSE/queue latency). */
export async function awaitRow(page, subject, timeoutMs = 60000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    if ((await listText(page)).includes(subject)) return true;
    if (Date.now() > deadline) return false;
    await page.waitForTimeout(1500);
  }
}

/** Cleanup helper that always runs, even when a scenario throws. */
export async function withCleanup(folders, fn) {
  try {
    return await fn();
  } finally {
    for (const f of folders) {
      try {
        await cleanupFolder(f);
      } catch (e) {
        console.log(`  (cleanup ${f} failed: ${e.message})`);
      }
    }
  }
}
