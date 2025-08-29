import { useCallback, useRef, useState } from "react";

export type MailToastLink = { label: string; folder: string };

export type MailToast = {
  id: number;
  label: string;
  onUndo?: () => void;
  // Pure-data deep link (e.g. "View sent" after a send): the shell resolves
  // the folder through the mail store, so no store callbacks cross hook
  // boundaries here.
  link?: MailToastLink;
};

/**
 * Undo-style toast state: one toast at a time, auto-dismissed after a
 * default of 5 seconds, with an optional undo callback and an optional
 * folder link rendered as an action button.
 */
export function useToast() {
  const [toast, setToast] = useState<MailToast | null>(null);
  const toastTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const showToast = useCallback(
    (label: string, onUndo?: () => void, duration = 5000, link?: MailToastLink) => {
      if (toastTimer.current) clearTimeout(toastTimer.current);
      setToast({ id: Date.now(), label, onUndo, link });
      toastTimer.current = setTimeout(() => setToast(null), duration);
    },
    [],
  );

  return { toast, setToast, showToast };
}
