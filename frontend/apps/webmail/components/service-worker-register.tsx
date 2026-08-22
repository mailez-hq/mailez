"use client";

import { useEffect } from "react";

/** Registers the offline-first service worker in production only. */
export function ServiceWorkerRegister() {
  useEffect(() => {
    if (process.env.NODE_ENV !== "production") return;
    if (!("serviceWorker" in navigator)) return;
    navigator.serviceWorker.register("/sw.js").catch(() => {
      // SW registration is best-effort; the app works without it.
    });
  }, []);
  return null;
}
