import { createViewCache } from "@/lib/view-cache";
import type { MailMessage, MailThread } from "@/lib/api";

// Module-level view caches shared by the store's reading-pane paths: the
// detail cache serves the pathId effect in mail-store and the flag actions
// (which invalidate entries), the thread cache serves the conversation
// preload. They live for the whole browser session, mirroring the former
// module-level singletons in mail-store.tsx.
export const detailCache = createViewCache<MailMessage>();
export const threadCache = createViewCache<MailThread>();
