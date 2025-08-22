/* mailez offline-first service worker */
const VERSION = "mailez-sw-v1";
const SHELL_URL = "/";

self.addEventListener("install", () => {
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== VERSION).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("fetch", (event) => {
  const { request } = event;
  if (request.method !== "GET") return;

  const url = new URL(request.url);
  if (url.origin !== self.location.origin) return;

  // Never cache API responses — they carry session data.
  if (url.pathname.startsWith("/api/")) return;

  // Versioned static assets: cache-first, with network refresh in background.
  if (url.pathname.startsWith("/_next/static/")) {
    event.respondWith(
      caches.match(request).then((hit) => {
        if (hit) return hit;
        return fetch(request).then((res) => {
          const copy = res.clone();
          caches.open(VERSION).then((cache) => cache.put(request, copy));
          return res;
        });
      }),
    );
    return;
  }

  // Navigations: network-first, fall back to the cached app shell.
  if (request.mode === "navigate") {
    event.respondWith(
      fetch(request)
        .then((res) => {
          const copy = res.clone();
          caches.open(VERSION).then((cache) => cache.put(SHELL_URL, copy));
          return res;
        })
        .catch(() => caches.match(SHELL_URL)),
    );
    return;
  }

  // Everything else (icons, manifest): stale-while-revalidate.
  event.respondWith(
    caches.match(request).then((hit) => {
      const fresh = fetch(request)
        .then((res) => {
          const copy = res.clone();
          caches.open(VERSION).then((cache) => cache.put(request, copy));
          return res;
        })
        .catch(() => hit);
      return hit || fresh;
    }),
  );
});
