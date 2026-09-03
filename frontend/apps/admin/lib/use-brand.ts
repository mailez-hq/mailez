"use client";

// Client hook for white-label wordmarks (login card, sidebar). One shared
// fetch per page load; the result is memoized module-level so every
// consumer renders the same title without extra requests. Server modules
// must import from branding.ts instead — Turbopack rejects react hooks in
// the server graph.
import { useEffect, useState } from "react";
import type { PublicBrand } from "@/lib/branding";

const NO_BRAND: PublicBrand = { title: "", logo_url: "" };

let memoized: Promise<PublicBrand> | null = null;

function brandPromise(): Promise<PublicBrand> {
  if (!memoized) {
    memoized = fetch("/api/v1/server/settings")
      .then((r) => (r.ok ? r.json() : null))
      .then((d) => {
        const title = String(d?.branding?.title ?? "").trim();
        return title
          ? { title, logo_url: String(d?.branding?.logo_url ?? "").trim() }
          : NO_BRAND;
      })
      .catch(() => NO_BRAND);
  }
  return memoized;
}

export function useBrand(): PublicBrand {
  const [brand, setBrand] = useState<PublicBrand>(NO_BRAND);
  useEffect(() => {
    let alive = true;
    brandPromise().then((b) => {
      if (alive) setBrand(b);
    });
    return () => {
      alive = false;
    };
  }, []);
  return brand;
}
