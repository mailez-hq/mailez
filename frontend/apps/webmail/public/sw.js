/* mailez offline-first service worker */
const VERSION = "mailez-sw-v2";
const SHELL_URL = "/";
const PRECACHE = [SHELL_URL, "/mailez-icon.svg", "/manifest.webmanifest"];

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches
      .open(VERSION)
      .then((cache) => cache.addAll(PRECACHE))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== VERSION).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

// Incoming push (new mail notifications). The payload is JSON:
// { title, body, url, tag }.
self.addEventListener("push", (event) => {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch {
    // non-JSON payload; show a generic notification
  }
  const d = data as { title?: string; body?: string; url?: string; tag?: string };
  event.waitUntil(
    self.registration.showNotification(d.title || "Mailez", {
      body: d.body || "",
      icon: "/mailez-icon.svg",
      badge: "/mailez-icon.svg",
      tag: d.tag || "mail",
      data: { url: d.url || "/" },
    }),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url = (event.notification.data && event.notification.data.url) || "/";
  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((clients) => {
      for (const client of clients) {
        if ("focus" in client) {
          client.navigate?.(url);
          return client.focus();
        }
      }
      return self.clients.openWindow(url);
    }),
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
