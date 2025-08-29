// ---- in-memory view cache ----
// Switching between conversations should be instant: keep recently fetched
// message details and threads in memory for a short TTL. Flag changes are
// bounded by the TTL; opening a message invalidates its entry (marks read).

export type ViewCacheEntry<T> = { at: number; data: T };

export const VIEW_CACHE_TTL = 30_000;
export const VIEW_CACHE_MAX = 120;

export type ViewCache<T> = Map<string, ViewCacheEntry<T>>;

export function createViewCache<T>(): ViewCache<T> {
  return new Map();
}

export function viewCacheGet<T>(m: ViewCache<T>, key: string): T | null {
  const e = m.get(key);
  if (!e) return null;
  if (Date.now() - e.at > VIEW_CACHE_TTL) {
    m.delete(key);
    return null;
  }
  return e.data;
}

export function viewCachePut<T>(m: ViewCache<T>, key: string, data: T) {
  if (m.size >= VIEW_CACHE_MAX) {
    let oldestKey: string | null = null;
    let oldestAt = Infinity;
    for (const [k, v] of m) {
      if (v.at < oldestAt) {
        oldestAt = v.at;
        oldestKey = k;
      }
    }
    if (oldestKey) m.delete(oldestKey);
  }
  m.set(key, { at: Date.now(), data });
}
