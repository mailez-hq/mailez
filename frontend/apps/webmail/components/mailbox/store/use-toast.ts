import { useCallback, useRef, useState } from "react";

export type MailToast = { id: number; label: string; onUndo?: () => void };

/**
 * Undo-style toast state: one toast at a time, auto-dismissed after a
 * default of 5 seconds, with an optional undo callback fired from the toast.
 */
export function useToast() {
  const [toast, setToast] = useState<MailToast | null>(null);
  const toastTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const showToast = useCallback((label: string, onUndo?: () => void, duration = 5000) => {
    if (toastTimer.current) clearTimeout(toastTimer.current);
    setToast({ id: Date.now(), label, onUndo });
    toastTimer.current = setTimeout(() => setToast(null), duration);
  }, []);

  return { toast, setToast, showToast };
}
