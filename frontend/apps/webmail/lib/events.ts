"use client";

// Server-Sent Events subscription for mailbox changes. The backend pushes a
// "ready" event when the stream (re)connects and a "mail" event when new mail
// arrives in a watched folder; EventSource reconnects automatically and the
// ready event re-baselines the client so nothing is missed while offline.

export type MailPushEvent = {
  type: string;
  folders?: string[];
  at?: string;
};

export type MailEventHandlers = {
  onReady?: () => void;
  onMail?: (ev: MailPushEvent) => void;
};

export function subscribeMailEvents(handlers: MailEventHandlers): () => void {
  if (typeof window === "undefined" || !("EventSource" in window)) {
    return () => {};
  }

  let es: EventSource | null = null;
  let closed = false;

  const open = () => {
    if (closed) return;
    es = new EventSource("/api/v1/events");
    es.addEventListener("ready", () => handlers.onReady?.());
    es.addEventListener("mail", (ev) => {
      try {
        handlers.onMail?.(JSON.parse((ev as MessageEvent).data) as MailPushEvent);
      } catch {
        // malformed event; ignore and wait for the next one
      }
    });
    // onerror: EventSource reconnects on its own; the ready event on the new
    // connection re-baselines the list.
  };

  open();
  return () => {
    closed = true;
    if (es) {
      es.close();
      es = null;
    }
  };
}
