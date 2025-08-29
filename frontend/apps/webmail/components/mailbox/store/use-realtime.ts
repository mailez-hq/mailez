"use client";

import { useEffect, useRef, useState } from "react";

import { setupPushSubscription, teardownPushSubscription } from "@/lib/push";
import { subscribeMailEvents } from "@/lib/events";

/**
 * Realtime cluster: browser online/offline status, the Web Push subscription
 * that follows the notification preference, and the SSE subscription for
 * live mailbox updates. The current folder is read through a ref so folder
 * switches don't tear down and recreate the connection.
 */
export function useRealtime({
  email,
  notifications,
  folder,
  refreshUnseen,
  loadMessages,
}: {
  email: string;
  notifications: boolean;
  folder: string;
  refreshUnseen: () => void;
  loadMessages: (folder: string, page?: number, silent?: boolean) => Promise<void>;
}) {
  const [online, setOnline] = useState(() =>
    typeof navigator !== "undefined" ? navigator.onLine : true,
  );

  const folderRef = useRef(folder);
  // Keep the folder ref current after every render (writing refs during
  // render is a React 19 anti-pattern); the SSE callbacks read it so folder
  // switches don't tear down and recreate the connection.
  useEffect(() => {
    folderRef.current = folder;
  });

  // Register/unregister the push subscription with the backend when the
  // notification preference changes.
  useEffect(() => {
    if (!email) return;
    if (notifications) {
      setupPushSubscription().catch(() => {});
    } else {
      teardownPushSubscription().catch(() => {});
    }
  }, [email, notifications]);

  // Real-time mailbox updates: the backend pushes a "mail" event over SSE when
  // new mail arrives in a watched folder. The current folder is read through a
  // ref so switching folders doesn't tear down and recreate the connection.
  useEffect(() => {
    if (!email) return;
    return subscribeMailEvents({
      onReady: () => {
        // (Re)connected: pick up anything that arrived while offline.
        refreshUnseen();
        void loadMessages(folderRef.current, 0, true);
      },
      onMail: (ev) => {
        refreshUnseen();
        const folders = ev.folders || [];
        const cur = folderRef.current;
        if (
          folders.length === 0 ||
          folders.some((f) => f.toLowerCase() === cur.toLowerCase())
        ) {
          void loadMessages(cur, 0, true);
        }
      },
    });
  }, [email, refreshUnseen, loadMessages]);

  // connection status banner (initial value seeds navigator.onLine where it
  // exists; the effect only subscribes to changes — no state writes).
  useEffect(() => {
    if (typeof navigator === "undefined") return;
    const on = () => setOnline(true);
    const off = () => setOnline(false);
    window.addEventListener("online", on);
    window.addEventListener("offline", off);
    return () => {
      window.removeEventListener("online", on);
      window.removeEventListener("offline", off);
    };
  }, []);

  return { online };
}
