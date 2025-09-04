"use client";

import { useEffect, useRef } from "react";

import { playChime } from "@/components/mailbox/mail-utils";
import { mailUnseen } from "@/lib/api";
import { useBrand } from "@/lib/use-brand";

// Polls the unseen counts and, when the Inbox count grows while the tab is
// not focused, rings a chime and raises a desktop notification. The system
// notification is titled with the configured brand (white-label), falling
// back to the built-in Mailez name only when nothing is branded.
export function useNewMailNotification(
  enabled: boolean,
  t: (key: string, values?: Record<string, string | number | Date>) => string,
) {
  const lastInboxUnseen = useRef<number | null>(null);
  // Module-level memo inside useBrand: no extra request, same data as the
  // sidebar wordmark.
  const { title: brandTitle } = useBrand();
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
                new Notification(brandTitle || "Mailez", { body: t("newMail", { count: n - prev }) });
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
  }, [enabled, t, brandTitle]);
}
