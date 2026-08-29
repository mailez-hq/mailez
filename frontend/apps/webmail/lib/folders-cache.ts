import { mailFolders } from "./api";

// Folder-list stale-while-revalidate cache, keyed per account. The folder
// sidebar is the one region of the mailbox that used to paint empty after
// login and pop in a beat later: the store only mounts after the session
// probe resolves, so its first /mail/folders round trip always landed after
// the rest of the chrome. Two fixes share this module:
//
//   - the login screen prefetches the list into this cache the moment the
//     session is granted, overlapping the navigation/route-load window;
//   - the store hydrates from the cache on mount (previous session's list)
//     and revalidates in the background, so the sidebar paints populated
//     on the very first frame.
//
// The cache is per account email, so one browser used by two accounts never
// shows the other account's folders; a stale list after a crash or offline
// exit is corrected by the revalidate.
const keyFor = (email: string) => `mailez.folders.${email.trim().toLowerCase()}`;

export function readCachedFolders(email: string): string[] | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = window.localStorage.getItem(keyFor(email));
    if (!raw) return null;
    const v: unknown = JSON.parse(raw);
    if (Array.isArray(v) && v.every((x) => typeof x === "string")) {
      return v as string[];
    }
  } catch {
    // malformed or unavailable storage: treat as a miss
  }
  return null;
}

export function writeCachedFolders(email: string, folders: string[]): void {
  if (typeof window === "undefined" || folders.length === 0) return;
  try {
    window.localStorage.setItem(keyFor(email), JSON.stringify(folders));
  } catch {
    // storage full or blocked: the in-memory list still works
  }
}

// prefetchFolders warms the cache right after a successful login, while the
// router is still navigating to the mailbox. Fire-and-forget by design.
export function prefetchFolders(email: string): void {
  if (typeof window === "undefined") return;
  mailFolders()
    .then((folders) => writeCachedFolders(email, folders))
    .catch(() => {
      // expired or revoked mid-flight: the store's own load will surface it
    });
}
