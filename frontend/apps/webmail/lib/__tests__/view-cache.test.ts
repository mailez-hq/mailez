import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createViewCache, viewCacheGet, viewCachePut, VIEW_CACHE_MAX } from "@/lib/view-cache";

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("viewCache", () => {
  it("returns null on a miss", () => {
    const cache = createViewCache<string>();
    expect(viewCacheGet(cache, "missing")).toBeNull();
  });

  it("returns data within the TTL", () => {
    const cache = createViewCache<string>();
    viewCachePut(cache, "k", "v");
    vi.advanceTimersByTime(10_000);
    expect(viewCacheGet(cache, "k")).toBe("v");
  });

  it("expires entries beyond the TTL and evicts them on read", () => {
    const cache = createViewCache<string>();
    viewCachePut(cache, "k", "v");
    vi.advanceTimersByTime(31_000);
    expect(viewCacheGet(cache, "k")).toBeNull();
    expect(cache.has("k")).toBe(false);
  });

  it("evicts the oldest entry when the cache is full", () => {
    const cache = createViewCache<number>();
    viewCachePut(cache, "first", 1);
    vi.advanceTimersByTime(1_000);
    for (let i = 0; i < VIEW_CACHE_MAX - 1; i++) {
      viewCachePut(cache, `filler-${i}`, i);
    }
    expect(cache.size).toBe(VIEW_CACHE_MAX);
    // One more put must evict "first" (the oldest), then insert itself.
    viewCachePut(cache, "newest", -1);
    expect(cache.has("first")).toBe(false);
    expect(cache.size).toBe(VIEW_CACHE_MAX);
    expect(viewCacheGet(cache, "newest")).toBe(-1);
  });

  it("refreshes the timestamp when a key is overwritten", () => {
    const cache = createViewCache<string>();
    viewCachePut(cache, "k", "old");
    vi.advanceTimersByTime(20_000);
    viewCachePut(cache, "k", "new");
    vi.advanceTimersByTime(20_000);
    expect(viewCacheGet(cache, "k")).toBe("new");
  });
});
