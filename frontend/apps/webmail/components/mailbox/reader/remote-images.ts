// Remote-image policy helpers: detection, blocking and per-sender memory.

const REMOTE_SENDERS_KEY = "mailez.remoteSenders";

export function hasRemoteImages(html?: string) {
  return /<img\b[^>]*\bsrc="https?:\/\//i.test(html || "");
}

export function rememberedRemoteSenders(): string[] {
  try {
    const raw = localStorage.getItem(REMOTE_SENDERS_KEY);
    if (raw) return JSON.parse(raw);
  } catch {
    // storage unavailable
  }
  return [];
}

export function rememberRemoteSender(domain: string) {
  const list = rememberedRemoteSenders();
  if (!list.includes(domain)) {
    list.push(domain);
    try {
      localStorage.setItem(REMOTE_SENDERS_KEY, JSON.stringify(list));
    } catch {
      // storage unavailable
    }
  }
}

export function blockRemoteImages(html: string) {
  return html.replace(
    /(<img\b[^>]*\bsrc)="(https?:\/\/[^"]+)"/gi,
    '$1="about:blank" data-remote-src="$2"',
  );
}
