"use client";

import { useCallback, useRef, useState, type Dispatch, type RefObject, type SetStateAction } from "react";

import { mailMessages, type MailMessage } from "@/lib/api";

/**
 * Message-list cluster: the loaded folder page set (messages/total/page),
 * the load/loading/cursor/selection state and the sort parameters. Owns the
 * load-sequence guard that drops superseded folder loads; search owns an
 * independent counter and bumps this one when it takes over the list.
 */
export function useMailList({
  folder,
  conversation,
  setError,
}: {
  folder: string;
  conversation: boolean;
  setError: Dispatch<SetStateAction<string>>;
}) {
  const [messages, setMessages] = useState<MailMessage[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [cursor, setCursor] = useState(0);
  const [selectedUids, setSelectedUids] = useState<Set<number>>(new Set());
  const [sortBy, setSortBy] = useState("date");
  const [sortDir, setSortDir] = useState("desc");
  const [refreshing, setRefreshing] = useState(false);
  const loadingMoreRef = useRef(false);
  // Guards loadMessages responses: increments on every call so an older
  // in-flight request can detect it has been superseded and drop its result.
  // (Search responses have their own independent counter inside useMailSearch,
  // which also invalidates this one on every search.)
  const loadSeq = useRef(0);

  const loadMessages = useCallback(async (f: string, p = 0, silent = false) => {
    // A monotonically increasing seq lets stale responses drop themselves: the
    // mount effect loads the initial folder before the pathname effect has
    // corrected it, so two requests race and the slower (older) one must not
    // overwrite the current folder's list.
    const seq = ++loadSeq.current;
    if (p === 0 && !silent) setLoading(true);
    try {
      const res = await mailMessages(f, p, sortBy, sortDir, conversation);
      if (seq !== loadSeq.current) return;
      setMessages(p === 0 ? res.messages : (prev) => [...prev, ...res.messages]);
      setTotal(res.total);
      setPage(p);
    } catch (e) {
      if (seq !== loadSeq.current) return;
      setError(e instanceof Error ? e.message : "load messages failed");
    } finally {
      if (seq === loadSeq.current) setLoading(false);
    }
  }, [sortBy, sortDir, conversation, setError]);

  // changeSort updates the list ordering (date desc/asc, by sender, subject
  // or size) and reloads the current folder. The immediate reload races with
  // the identity change of loadMessages (which re-triggers the folder effect
  // with the new sort); the sequence guard drops whichever lands second.
  function changeSort(key: string) {
    const [s, d] = key.split("-");
    const nextBy = s || "date";
    const nextDir = d === "asc" ? "asc" : nextBy === "date" ? "desc" : "asc";
    setSortBy(nextBy);
    setSortDir(nextDir);
    loadMessages(folder, 0, true);
  }

  function toggleSelect(m: MailMessage) {
    setSelectedUids((prev) => {
      const next = new Set(prev);
      if (next.has(m.uid)) next.delete(m.uid);
      else next.add(m.uid);
      return next;
    });
  }

  return {
    messages, setMessages,
    total, setTotal, page, loading,
    cursor, setCursor,
    selectedUids, setSelectedUids,
    sortBy, sortDir,
    refreshing, setRefreshing,
    loadingMoreRef, loadSeq,
    loadMessages, changeSort, toggleSelect,
  };
}

export type MailList = ReturnType<typeof useMailList>;
export type LoadMessages = ReturnType<typeof useMailList>["loadMessages"];
export type LoadSeqRef = RefObject<number>;
