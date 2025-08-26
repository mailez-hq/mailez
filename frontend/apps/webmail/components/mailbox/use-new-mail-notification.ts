"use client";

import { useEffect, useRef } from "react";

import { playChime } from "@/components/mailbox/mail-utils";
import { mailUnseen } from "@/lib/api";

// Polls the unseen counts and, when the Inbox count grows while the tab is
// not focused, rings a chime and raises a desktop notification.
export function useNewMailNotification(
  enabled: boolean,
  t: (key: string, values?: Record<string, string | number | Date>) => string,
) {
  const lastInboxUnseen = useRef<number | null>(null);
  useEffect(() => {
    if (!enabled) return;
    const check = () => {
      mailUnseen()
        .then((counts) => {
          const n = counts["Inbox"] ?? 0;
          const prev = lastInboxUnseen.current;
          lastInboxUnseen.current = n;
          if (prev !== null && n > prev && !document.hasFocus()) {
            playChime();
            if (typeof Notification !== "undefined" && Notification.permission === "granted") {
              try {
                new Notification("Mailez", { body: t("newMail", { count: n - prev }) });
              } catch {
                // notification rejected by the platform
              }
            }
          }
        })
        .catch(() => {});
    };
    check();
    const id = setInterval(check, 60000);
    return () => clearInterval(id);
  }, [enabled, t]);
}
