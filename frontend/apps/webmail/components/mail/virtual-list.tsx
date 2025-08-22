"use client";

import { useEffect, useRef, useState } from "react";
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
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportH, setViewportH] = useState(0);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const update = () => setViewportH(el.clientHeight);
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  useEffect(() => {
    containerRef.current?.scrollTo({ top: 0 });
    setScrollTop(0);
  }, [scrollKey]);

  const start = Math.max(0, Math.floor(scrollTop / rowHeight) - overscan);
  const end = Math.min(
    items.length,
    Math.ceil((scrollTop + viewportH) / rowHeight) + overscan,
  );

  // Fire onEndReached once whenever the tail of the list becomes visible.
  const nearEnd = end >= items.length && items.length > 0;
  const onEndReachedRef = useRef(onEndReached);
  onEndReachedRef.current = onEndReached;
  useEffect(() => {
    if (nearEnd) onEndReachedRef.current?.();
  }, [nearEnd, items.length]);

  return (
    <div
      ref={containerRef}
      onScroll={(e) => setScrollTop(e.currentTarget.scrollTop)}
      className={cn("mail-scroll overflow-y-auto", className)}
    >
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
