"use client";

import { useSyncExternalStore } from "react";

// useMounted defers HTML-body rendering until hydration completes so untrusted
// email HTML is only ever produced by DOMPurify in the browser, never by SSR.
// Expressed through useSyncExternalStore: the server snapshot is false, the
// client snapshot true — no effect, no extra render pass.
const subscribeNoop = () => () => {};

export function useMounted() {
  return useSyncExternalStore(
    subscribeNoop,
    () => true,
    () => false,
  );
}
