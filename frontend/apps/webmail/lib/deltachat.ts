// Delta Chat login QR support: the DCLOGIN v1 URI scheme as parsed by
// deltachat-core (src/qr/dclogin_scheme.rs). The QR embeds the account
// address, a one-time app token as the password and the public IMAP/SMTP
// settings so a Delta Chat client configures itself from a single scan:
//
//   DCLOGIN:user@example.org?p=<token>&v=1&ih=mail.example.org&ip=993&is=ssl&sh=…&sp=465&ss=ssl
//
// Server settings come from the deployment's own autoconfig XML
// (/mail/config-v1.1.xml, proxied by both frontends and the gateway), so the
// QR always agrees with what Thunderbird/Outlook autodiscovery hands out.

export type DcServerSettings = {
  imapHost: string;
  imapPort: number;
  imapSecurity: "ssl" | "starttls";
  smtpHost: string;
  smtpPort: number;
  smtpSecurity: "ssl" | "starttls";
};

const fallbackSettings = (): DcServerSettings => ({
  // Gateway-less and unresolvable cases: the webmail origin is the mail host
  // in every standard deployment; implicit TLS ports are always served.
  imapHost: typeof window !== "undefined" ? window.location.hostname : "",
  imapPort: 993,
  imapSecurity: "ssl",
  smtpHost: typeof window !== "undefined" ? window.location.hostname : "",
  smtpPort: 465,
  smtpSecurity: "ssl",
});

const socketType = (raw: string | null | undefined): "ssl" | "starttls" => {
  const v = (raw || "").toUpperCase();
  return v === "STARTTLS" ? "starttls" : "ssl"; // SSL is the default everywhere
};

// fetchDcServerSettings parses the deployment autoconfig XML. Never throws:
// any failure keeps the fallback so the QR generation flow stays alive.
export async function fetchDcServerSettings(): Promise<DcServerSettings> {
  const fallback = fallbackSettings();
  try {
    const res = await fetch("/mail/config-v1.1.xml", { headers: { Accept: "application/xml" } });
    if (!res.ok) return fallback;
    const text = await res.text();
    const doc = new DOMParser().parseFromString(text, "application/xml");
    const incoming = Array.from(doc.getElementsByTagName("incomingServer")).find(
      (el) => el.getAttribute("type") === "imap",
    );
    const outgoing = Array.from(doc.getElementsByTagName("outgoingServer")).find(
      (el) => el.getAttribute("type") === "smtp",
    );
    if (!incoming || !outgoing) return fallback;
    const imapPort = Number(incoming.getElementsByTagName("port")[0]?.textContent) || 993;
    const smtpPort = Number(outgoing.getElementsByTagName("port")[0]?.textContent) || 465;
    return {
      imapHost: incoming.getElementsByTagName("hostname")[0]?.textContent?.trim() || fallback.imapHost,
      imapPort,
      imapSecurity: socketType(incoming.getElementsByTagName("socketType")[0]?.textContent),
      smtpHost: outgoing.getElementsByTagName("hostname")[0]?.textContent?.trim() || fallback.smtpHost,
      smtpPort,
      smtpSecurity: socketType(outgoing.getElementsByTagName("socketType")[0]?.textContent),
    };
  } catch {
    return fallback;
  }
}

// buildDcLoginUri renders the DCLOGIN v1 URI. Values are percent-encoded the
// way url::Url::query_pairs decodes them (standard encodeURIComponent).
export function buildDcLoginUri(
  email: string,
  password: string,
  s: DcServerSettings,
): string {
  const q = new URLSearchParams({
    p: password,
    v: "1",
    ih: s.imapHost,
    ip: String(s.imapPort),
    is: s.imapSecurity,
    sh: s.smtpHost,
    sp: String(s.smtpPort),
    ss: s.smtpSecurity,
  });
  return `DCLOGIN:${email}?${q.toString()}`;
}
