"use client";

import { useEffect, useState } from "react";

// useMounted defers HTML-body rendering until hydration completes so untrusted
// email HTML is only ever produced by DOMPurify in the browser, never by SSR.
export function useMounted() {
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);
  return mounted;
}
