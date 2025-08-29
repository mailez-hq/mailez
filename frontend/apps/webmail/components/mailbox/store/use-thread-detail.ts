"use client";

import { useEffect, useState } from "react";

import { mailMessage, mailThread, type MailMessage, type MailThread } from "@/lib/api";
import { viewCacheGet, viewCachePut } from "@/lib/view-cache";
import { threadCache } from "./view-caches";

/**
 * Reading-pane cluster: the opened message (selected/detail + loading) and
 * its conversation (thread view). Owns the conversation preload/fetch that
 * auto-opens thread view; the URL-driven detail load (pathId effect)
 * stays in mail-store because it also resets the AI summary.
 */
export function useThreadDetail({
  folder,
  router,
  conversation,
  setError,
}: {
  folder: string;
  router: { push: (href: string) => void };
  conversation: boolean;
  setError: (value: string | ((prev: string) => string)) => void;
}) {
  const [selected, setSelected] = useState<MailMessage | null>(null);
  const [detail, setDetail] = useState<MailMessage | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [thread, setThread] = useState<MailThread | null>(null);
  const [threadOpen, setThreadOpen] = useState(false);
  const [threadLoading, setThreadLoading] = useState(false);

  // Auto-load the conversation when a message that belongs to a thread
  // opens, so the reading pane shows the thread view without any
  // extra click. The previous conversation is dropped immediately so
  // switching between threads never renders stale members. The drop and any
  // cache hit run during render (React-endorsed adjustment); only the
  // network fetch stays in the effect.
  const threadKeySig = `${folder}\x00${detail?.thread_id ?? ""}\x00${conversation ? 1 : 0}`;
  const [prevThreadKeySig, setPrevThreadKeySig] = useState(threadKeySig);
  if (threadKeySig !== prevThreadKeySig) {
    setPrevThreadKeySig(threadKeySig);
    setThread(null);
    setThreadOpen(false);
    if (conversation && detail?.thread_id) {
      const threadKey = `${folder}\x00${detail.thread_id}`;
      const cachedThread = viewCacheGet(threadCache, threadKey);
      if (cachedThread) {
        setThread(cachedThread);
        setThreadOpen(true);
        setThreadLoading(false);
      } else {
        setThreadLoading(true);
      }
    }
  }

  useEffect(() => {
    if (!conversation || !detail?.thread_id) {
      return;
    }
    const threadKey = `${folder}\x00${detail.thread_id}`;
    if (viewCacheGet(threadCache, threadKey)) return;
    let cancelled = false;
    mailThread(folder, detail.thread_id)
      .then((th) => {
        if (cancelled) return;
        viewCachePut(threadCache, threadKey, th);
        setThread(th);
        setThreadOpen(true);
      })
      .catch(() => {
        if (!cancelled) setThread(null);
      })
      .finally(() => {
        if (!cancelled) setThreadLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [detail?.thread_id, folder, conversation]);

  async function toggleThread() {
    if (!detail?.thread_id) return;
    if (threadOpen && thread?.thread_id === detail.thread_id) {
      setThreadOpen(false);
      return;
    }
    setThreadOpen(true);
    if (!thread || thread.thread_id !== detail.thread_id) {
      setThreadLoading(true);
      setError("");
      try {
        setThread(await mailThread(folder, detail.thread_id));
      } catch (e) {
        setError(e instanceof Error ? e.message : "thread failed");
      } finally {
        setThreadLoading(false);
      }
    }
  }

  async function selectThreadMessage(uid: number) {
    if (!detail) return;
    setError("");
    try {
      const next = await mailMessage(folder, { uid });
      setDetail(next);
      // Keep the URL pinned to the opened thread member so it stays shareable.
      router.push(`/mail/${encodeURIComponent(folder)}/${next.id || next.uid}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load message failed");
    }
  }

  function backToList() {
    // Navigate back to the folder; the pathId effect clears the reading pane.
    router.push(`/mail/${encodeURIComponent(folder)}`);
    setSelected(null);
    setDetail(null);
    setThreadOpen(false);
  }

  return {
    selected, setSelected,
    detail, setDetail,
    detailLoading, setDetailLoading,
    thread, setThread, threadOpen, setThreadOpen, threadLoading,
    toggleThread, selectThreadMessage, backToList,
  };
}
