"use client";

import { useEffect, useRef, useState } from "react";
import { RefreshCw } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * Fixed-row-height virtual list. Renders only the visible window plus an
 * overscan, keeping huge mailboxes (10k+ messages) smooth without any
 * third-party dependency.
 */
export function VirtualList<T>({
  items,
  rowHeight,
  renderRow,
  getKey,
  className,
  overscan = 8,
  onEndReached,
  scrollKey,
  onPullRefresh,
}: {
  items: T[];
  rowHeight: number;
  renderRow: (item: T, index: number) => React.ReactNode;
  getKey: (item: T, index: number) => React.Key;
  className?: string;
  overscan?: number;
  onEndReached?: () => void;
  /** When this value changes the scroll position resets to the top. */
  scrollKey?: string;
  /** Mobile pull-to-refresh callback (fires after the list is pulled down). */
  onPullRefresh?: () => void;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportH, setViewportH] = useState(0);
  const [pull, setPull] = useState(0);
  const pullRef = useRef({ start: 0, active: false });

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const update = () => setViewportH(el.clientHeight);
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Reset the scroll position synchronously whenever the list is re-keyed
  // (render-phase adjustment — the DOM scroll below still happens in the
  // effect, which has no state writes).
  const [prevScrollKey, setPrevScrollKey] = useState(scrollKey);
  if (scrollKey !== prevScrollKey) {
    setPrevScrollKey(scrollKey);
    setScrollTop(0);
  }

  useEffect(() => {
    containerRef.current?.scrollTo({ top: 0 });
  }, [scrollKey]);

  const start = Math.max(0, Math.floor(scrollTop / rowHeight) - overscan);
  const end = Math.min(
    items.length,
    Math.ceil((scrollTop + viewportH) / rowHeight) + overscan,
  );

  // Fire onEndReached once whenever the tail of the list becomes visible.
  const nearEnd = end >= items.length && items.length > 0;
  const onEndReachedRef = useRef(onEndReached);
  useEffect(() => {
    onEndReachedRef.current = onEndReached;
  }, [onEndReached]);
  useEffect(() => {
    if (nearEnd) onEndReachedRef.current?.();
  }, [nearEnd, items.length]);

  return (
    <div
      ref={containerRef}
      onScroll={(e) => setScrollTop(e.currentTarget.scrollTop)}
      onTouchStart={(e) => {
        if (!onPullRefresh || !containerRef.current || containerRef.current.scrollTop > 0) return;
        pullRef.current = { start: e.touches[0].clientY, active: true };
      }}
      onTouchMove={(e) => {
        if (!pullRef.current.active) return;
        const dy = e.touches[0].clientY - pullRef.current.start;
        if (dy > 0) {
          if (dy > 8) e.preventDefault();
          setPull(Math.min(dy * 0.5, 110));
        } else {
          setPull(0);
        }
      }}
      onTouchEnd={() => {
        if (pullRef.current.active && pull > 70) onPullRefresh?.();
        pullRef.current.active = false;
        setPull(0);
      }}
      className={cn("mail-scroll relative overflow-y-auto", className)}
      style={pull > 0 ? { transform: `translateY(${pull}px)` } : undefined}
    >
      <div
        className="pointer-events-none absolute inset-x-0 top-0 z-10 flex justify-center pt-2"
        style={{ opacity: Math.min(pull / 70, 1) }}
      >
        <RefreshCw className={cn("size-4 text-primary", pull > 70 && "rotate-180")} />
      </div>
      <div
        style={{
          height: items.length * rowHeight,
          position: "relative",
        }}
      >
        {items.slice(start, end).map((item, i) => {
          const index = start + i;
          return (
            <div
              key={getKey(item, index)}
              style={{
                position: "absolute",
                top: index * rowHeight,
                height: rowHeight,
                left: 0,
                right: 0,
              }}
            >
              {renderRow(item, index)}
            </div>
          );
        })}
      </div>
    </div>
  );
}
