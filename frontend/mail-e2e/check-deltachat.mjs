// Delta Chat compatibility check: exercises the whole onboarding path a
// Delta Chat client would take — the autoconfig XML discovery, the webmail
// DCLOGIN QR flow (mint an app token from the settings dialog), and the raw
// protocol behaviour the chat client depends on: token-based IMAP LOGIN +
// concurrent IDLE connections and token-based SMTP AUTH. Runs against the
// local compose stack (webmail :8083, IMAP :143, SMTP :587).
import net from "node:net";
import tls from "node:tls";
import { chromium } from "playwright";
import { USER, apiLogin } from "./helper.mjs";

const BASE = "http://127.0.0.1:8083";
const IMAP = { host: "127.0.0.1", port: 143 };
const SMTP = { host: "127.0.0.1", port: 587 };
const results = [];
const check = (name, ok) => results.push(`${ok ? "PASS" : "FAIL"}  ${name}`);

// upgradeTls wraps an existing socket in TLS after a STARTTLS handshake —
// the local stack refuses cleartext authentication (correctly), exactly
// like a production deployment, so the check speaks real STARTTLS.
function upgradeTls(sock) {
  return tls.connect({ socket: sock, rejectUnauthorized: false, servername: "localhost" });
}

// --- 1. autoconfig XML (what a Delta Chat client fetches on manual domain
// entry; the DCLOGIN QR generator also reads it for host/port truth) ---
const acRes = await fetch(`${BASE}/mail/config-v1.1.xml`);
const acXml = await acRes.text();
check("autoconfig: xml served on webmail origin", acRes.ok && acXml.includes("<clientConfig"));
check("autoconfig: imap + smtp entries", acXml.includes('type="imap"') && acXml.includes('type="smtp"'));

// --- 2. webmail settings: Delta Chat section mints a token and renders a QR ---
const cookie = await apiLogin();
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
await page.context().addCookies([{ name: cookie.name, value: cookie.value, domain: "127.0.0.1", path: "/" }]);
await page.goto(`${BASE}/`, { waitUntil: "domcontentloaded" });
await page.waitForTimeout(4000);

// Open the settings dialog and switch to the Delta Chat section.
await page.getByRole("button", { name: /设置|Settings/ }).first().click();
await page.waitForTimeout(1500);
const navBtn = page.locator("aside nav button", { hasText: /Delta Chat/i }).first();
await navBtn.click();
await page.waitForTimeout(800);
const generate = page.getByRole("button", { name: /登录二维码|login QR/i }).first();
const hasSection = (await generate.count()) > 0;
check("webmail: Delta Chat settings section", hasSection);

let token = "";
if (hasSection) {
  await generate.click();
  await page.waitForTimeout(3000);
  const qrImg = page.locator("img[alt='Delta Chat']");
  const tokenInput = page.locator("input.font-mono");
  const src = (await qrImg.first().getAttribute("src")) || "";
  const hasQr = (await qrImg.count()) > 0 && src.startsWith("data:image/png");
  token = ((await tokenInput.first().getAttribute("value")) || "").trim();
  check("webmail: QR rendered from generated token", hasQr && /^[0-9a-f]{32}$/.test(token));
}
await browser.close();

// --- 3. the minted token must authenticate the protocol path the chat
// client uses: STARTTLS then IMAP LOGIN / SMTP AUTH PLAIN (no webmail
// session involved — exactly what the Delta Chat core does). ---
function imapLogin(user, pass) {
  return new Promise((resolve, reject) => {
    const plain = net.createConnection(IMAP.port, IMAP.host);
    let buf = "";
    let stage = "greet";
    const t = setTimeout(() => { plain.destroy(); reject(new Error("timeout")); }, 15000);
    const fail = (msg) => { clearTimeout(t); plain.destroy(); reject(new Error(msg)); };
    plain.on("data", (d) => {
      buf += d.toString("utf8");
      if (stage === "greet" && buf.startsWith("* ")) {
        stage = "starttls";
        plain.write("a0 STARTTLS\r\n");
        return;
      }
      if (stage === "starttls" && buf.includes("a0 OK")) {
        stage = "tls";
        plain.removeAllListeners("data");
        const secure = upgradeTls(plain);
        secure.on("secureConnect", () => {
          secure.write(`a1 LOGIN ${user} ${pass}\r\n`);
        });
        secure.on("data", (sd) => {
          const s = sd.toString("utf8");
          if (s.includes("a1 OK")) {
            clearTimeout(t);
            secure.write("a2 LOGOUT\r\n");
            secure.end();
            resolve(s);
          } else if (s.includes("a1 NO") || s.includes("a1 BAD")) {
            clearTimeout(t);
            secure.destroy();
            reject(new Error(s.trim().split("\r\n").pop()));
          }
        });
        secure.on("error", (e) => { clearTimeout(t); reject(e); });
        return;
      }
      if (buf.includes("a0 NO") || buf.includes("a0 BAD")) fail(buf.trim().split("\r\n").pop());
    });
    plain.on("error", (e) => { clearTimeout(t); reject(e); });
  });
}

function smtpAuth(user, pass) {
  return new Promise((resolve, reject) => {
    const plain = net.createConnection(SMTP.port, SMTP.host);
    let buf = "";
    let stage = "greet";
    const t = setTimeout(() => { plain.destroy(); reject(new Error("timeout")); }, 15000);
    plain.on("data", (d) => {
      buf += d.toString("utf8");
      if (stage === "greet" && buf.startsWith("220")) {
        stage = "ehlo";
        plain.write("EHLO deltachat-check\r\n");
        return;
      }
      if (stage === "ehlo" && /\r?\n250 /.test(buf)) {
        stage = "starttls";
        plain.write("STARTTLS\r\n");
        return;
      }
      if (stage === "starttls" && buf.includes("220")) {
        stage = "tls";
        plain.removeAllListeners("data");
        const secure = upgradeTls(plain);
        secure.on("secureConnect", () => {
          secure.write("EHLO deltachat-check\r\n");
        });
        let tlsBuf = "";
        let tlsStage = "ehlo";
        secure.on("data", (sd) => {
          tlsBuf += sd.toString("utf8");
          if (tlsStage === "ehlo" && /\r?\n250 /.test(tlsBuf)) {
            tlsStage = "auth";
            const b64 = Buffer.from(`\u0000${user}\u0000${pass}`).toString("base64");
            secure.write(`AUTH PLAIN ${b64}\r\n`);
            return;
          }
          if (tlsStage === "auth") {
            if (tlsBuf.includes("235")) {
              clearTimeout(t);
              secure.write("QUIT\r\n");
              secure.end();
              resolve("235");
            } else if (/^5\d\d/.test(tlsBuf.trim().split("\r\n").pop() || "")) {
              clearTimeout(t);
              secure.destroy();
              reject(new Error(tlsBuf.trim().split("\r\n").pop()));
            }
          }
        });
        secure.on("error", (e) => { clearTimeout(t); reject(e); });
        return;
      }
      if (/^5\d\d/.test(buf.trim().split("\r\n").pop() || "")) {
        clearTimeout(t);
        plain.destroy();
        reject(new Error(buf.trim().split("\r\n").pop()));
      }
    });
    plain.on("error", (e) => { clearTimeout(t); reject(e); });
  });
}

if (token) {
  try {
    const out = await imapLogin(USER.email, token);
    check("imap: app token LOGIN accepted", out.includes("a1 OK"));
  } catch (e) {
    check("imap: app token LOGIN accepted", false);
    console.log(`      (${e.message})`);
  }
  try {
    await smtpAuth(USER.email, token);
    check("smtp: app token AUTH PLAIN accepted", true);
  } catch (e) {
    check("smtp: app token AUTH PLAIN accepted", false);
    console.log(`      (${e.message})`);
  }

  // --- 4. multi-device: five concurrent authenticated IDLE connections ---
  try {
    const idles = await Promise.all(
      Array.from({ length: 5 }, () =>
        new Promise((resolve, reject) => {
          const plain = net.createConnection(IMAP.port, IMAP.host);
          let buf = "";
          let stage = "greet";
          const t = setTimeout(() => { plain.destroy(); reject(new Error("timeout")); }, 15000);
          plain.on("data", (d) => {
            buf += d.toString("utf8");
            if (stage === "greet" && buf.startsWith("* ")) {
              stage = "starttls";
              plain.write("a0 STARTTLS\r\n");
              return;
            }
            if (stage === "starttls" && buf.includes("a0 OK")) {
              stage = "tls";
              plain.removeAllListeners("data");
              const secure = upgradeTls(plain);
              secure.on("secureConnect", () => {
                secure.write(`a1 LOGIN ${USER.email} ${token}\r\n`);
              });
              secure.on("data", (sd) => {
                const s = sd.toString("utf8");
                if (s.includes("a1 OK") && stage === "tls") {
                  stage = "selected";
                  secure.write("a2 SELECT INBOX\r\n");
                  return;
                }
                if (s.includes("a2 OK")) {
                  secure.write("a3 IDLE\r\n");
                  return;
                }
                if (s.includes("+ idling")) {
                  clearTimeout(t);
                  resolve(true);
                }
                if (s.includes("a1 NO") || s.includes("a2 NO") || s.includes("a3 NO")) {
                  clearTimeout(t);
                  secure.destroy();
                  reject(new Error("server rejected login/select/idle"));
                }
              });
              secure.on("error", (e) => { clearTimeout(t); reject(e); });
              return;
            }
            if (buf.includes("a0 NO")) {
              clearTimeout(t);
              plain.destroy();
              reject(new Error("starttls rejected"));
            }
          });
          plain.on("error", (e) => { clearTimeout(t); reject(e); });
        }),
      ),
    );
    check("imap: 5 concurrent IDLE connections", idles.length === 5);
  } catch (e) {
    check("imap: 5 concurrent IDLE connections", false);
    console.log(`      (${e.message})`);
  }
}

console.log(results.join("\n"));
process.exit(results.some((r) => r && r.startsWith("FAIL")) ? 1 : 0);
