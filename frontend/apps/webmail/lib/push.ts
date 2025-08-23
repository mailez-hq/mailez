"use client";

import { pushSubscribe, pushUnsubscribe, pushVapid } from "@/lib/api";

function bufToBase64Url(buf: ArrayBuffer | null): string {
  if (!buf) return "";
  const bytes = new Uint8Array(buf);
  let bin = "";
  for (let i = 0; i < bytes.length; i += 0x8000) {
    bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  }
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

// setupPushSubscription asks for notification permission (once) and registers
// the service-worker push subscription with the backend, so new mail can be
// pushed even when no tab is open.
export async function setupPushSubscription(): Promise<boolean> {
  if (typeof window === "undefined") return false;
  if (!("serviceWorker" in navigator) || !("PushManager" in window) || !("Notification" in window)) {
    return false;
  }
  if (Notification.permission === "default") {
    const ok = await Notification.requestPermission();
    if (ok !== "granted") return false;
  }
  if (Notification.permission !== "granted") return false;
  try {
    const reg = await navigator.serviceWorker.ready;
    const { public_key } = await pushVapid();
    const sub = await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: public_key,
    });
    await pushSubscribe({
      endpoint: sub.endpoint,
      keys: { p256dh: bufToBase64Url(sub.getKey("p256dh")), auth: bufToBase64Url(sub.getKey("auth")) },
    });
    return true;
  } catch {
    return false;
  }
}

export async function teardownPushSubscription(): Promise<void> {
  if (typeof window === "undefined" || !("serviceWorker" in navigator)) return;
  try {
    const reg = await navigator.serviceWorker.ready;
    const sub = await reg.pushManager.getSubscription();
    if (sub) {
      await pushUnsubscribe({ endpoint: sub.endpoint }).catch(() => {});
      await sub.unsubscribe().catch(() => {});
    }
  } catch {
    // best-effort
  }
}
