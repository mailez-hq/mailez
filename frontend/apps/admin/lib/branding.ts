// White-label brand resolution for surfaces the branding admin config does
// not reach through props: browser tab titles (metadata) and the PWA
// manifest read the public /server/settings branding block here. An
// unreachable backend or an empty title means "not branded" and callers
// keep the built-in Mailez brand. Server-safe: no react imports (the client
// hook lives in use-brand.ts).
export type PublicBrand = { title: string; logo_url: string };

// Server-side (metadata / manifest): the Next rewrite only serves browser
// traffic, so server code targets API_TARGET directly. `no-store` keeps the
// brand fresh per request; the timeout plus catch degrade to the default
// brand instead of failing rendering when the backend is briefly down.
export async function fetchPublicBrand(): Promise<PublicBrand | null> {
  const target = process.env.API_TARGET || "http://localhost:8080";
  try {
    const res = await fetch(`${target}/api/v1/server/settings`, {
      cache: "no-store",
      signal: AbortSignal.timeout(3000),
    });
    if (!res.ok) return null;
    const data = await res.json();
    const title = String(data?.branding?.title ?? "").trim();
    if (!title) return null;
    return { title, logo_url: String(data?.branding?.logo_url ?? "").trim() };
  } catch {
    return null;
  }
}
