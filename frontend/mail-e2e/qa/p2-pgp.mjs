// P2: PGP happy path. Generates a key for the test account, then runs the
// sign/verify and encrypt/decrypt round trips through the API the composer
// uses. Each call prints status + a short body so an unexpected payload shape
// is visible rather than silently reported as a failure.
// Run: cd frontend && node mail-e2e/qa/p2-pgp.mjs
import { USER, api, check, closeBrowser, dumpResults, record, summary } from "./lib.mjs";

const MOD = "P2-pgp";
const short = (r) => `status=${r.status} ${(r.text || "").slice(0, 110).replace(/\s+/g, " ")}`;
try {
  const before = await api("/api/v1/me/pgp");
  record(MOD, "PGP: status before", "PASS", short(before));

  if (before.json?.has_key !== true) {
    // Try the documented generator with no body first; if the handler wants a
    // passphrase the response says so and we retry with one.
    let gen = await api("/api/v1/me/pgp/generate", { method: "POST", body: {} });
    record(MOD, "PGP: generate key (no body)", gen.status < 400 ? "PASS" : "FAIL", short(gen));
    if (gen.status >= 400) {
      gen = await api("/api/v1/me/pgp/generate", { method: "POST", body: { passphrase: "" } });
      record(MOD, "PGP: generate key (passphrase:)", gen.status < 400 ? "PASS" : "FAIL", short(gen));
    }
  }
  const after = await api("/api/v1/me/pgp");
  check(MOD, "PGP: the account now has a key", after.json?.has_key === true, short(after));
  const keys = await api("/api/v1/me/pgp/keys");
  record(MOD, "PGP: key list", keys.status < 400 ? "PASS" : "FAIL", short(keys));

  // ---------------------------------------------------- sign + verify
  const text = "PGP round-trip probe";
  // The API is deliberately key-in-hand: callers pass the public key they got
  // from /me/pgp (own key) or /pgp/key (a recipient's, as the composer does).
  const publicKey = after.json?.public_key ?? before.json?.public_key ?? "";
  const signed = await api("/api/v1/mail/pgp/sign", { method: "POST", body: { text } });
  record(MOD, "PGP: sign", signed.status < 400 ? "PASS" : "FAIL", short(signed));
  const signature = signed.json?.signature ?? signed.json?.signed ?? "";
  if (signature) {
    const verified = await api("/api/v1/mail/pgp/verify", { method: "POST", body: { text, signature, public_key: publicKey } });
    check(MOD, "PGP: verify accepts the signature this account produced",
      verified.status < 400 && (verified.json?.valid === true || verified.json?.verified === true),
      short(verified));
  } else {
    record(MOD, "PGP: verify", "BLOCKED", "sign returned no signature field");
  }

  // ------------------------------------------------- encrypt + decrypt
  const enc = await api("/api/v1/mail/pgp/encrypt", { method: "POST", body: { text, public_key: publicKey } });
  record(MOD, "PGP: encrypt to the account's own key", enc.status < 400 ? "PASS" : "FAIL", short(enc));
  const cipher = enc.json?.encrypted ?? enc.json?.text ?? "";
  if (cipher) {
    const dec = await api("/api/v1/mail/pgp/decrypt", { method: "POST", body: { text: cipher } });
    // The handler answers with `plaintext` (not `text`).
    check(MOD, "PGP: decrypt returns the original plaintext",
      dec.status < 400 && String(dec.json?.plaintext ?? "").includes(text),
      short(dec));
  } else {
    record(MOD, "PGP: decrypt", "BLOCKED", "encrypt returned no ciphertext field");
  }
} finally {
  await closeBrowser();
}
const s = summary();
console.log(`\n=== p2-pgp: ${s.pass} passed, ${s.fail} failed, ${s.blocked} blocked (${s.total}) ===`);
await dumpResults("p2-pgp.json");
process.exit(s.fail > 0 ? 1 : 0);
