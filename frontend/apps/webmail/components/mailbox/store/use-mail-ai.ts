"use client";

import { useEffect, useMemo, useState, type Dispatch, type RefObject, type SetStateAction } from "react";

import { ApiError, aiPrioritize, aiSearch, aiStatus, aiSummarize, type MailMessage } from "@/lib/api";

/**
 * AI cluster: backend capability probe, the effective per-feature flags
 * (backend availability AND user toggles), the reading-pane summary,
 * priority-inbox sorting and natural-language search.
 */
export function useMailAi({
  prefsAiEnabled,
  prefsAi,
  detail,
  messages,
  query,
  setError,
  setMessages,
  setSearching,
  setSelected,
  setDetail,
  setCursor,
  searchSeqRef,
  loadSeq,
}: {
  prefsAiEnabled: boolean;
  prefsAi: { summary: boolean; draft: boolean; priority: boolean; search: boolean };
  detail: MailMessage | null;
  messages: MailMessage[];
  /** Current keyword-box text (owned by useMailSearch) used as the default AI query. */
  query: string;
  setError: Dispatch<SetStateAction<string>>;
  setMessages: Dispatch<SetStateAction<MailMessage[]>>;
  setSearching: Dispatch<SetStateAction<boolean>>;
  setSelected: Dispatch<SetStateAction<MailMessage | null>>;
  setDetail: Dispatch<SetStateAction<MailMessage | null>>;
  setCursor: Dispatch<SetStateAction<number>>;
  /** Same sequence guards the folder/search loaders use: an AI search
   * replaces the list, so in-flight folder loads / keyword searches must be
   * invalidated, and a late AI response must lose to whatever the user
   * started next. */
  searchSeqRef: RefObject<number>;
  loadSeq: RefObject<number>;
}) {
  const [aiEnabled, setAiEnabled] = useState(false);
  // aiLocked marks the deployment without the AI module (the /ai/status route
  // itself is absent, a 404): the entry points render a visible locked hint
  // instead of silently disappearing.
  const [aiLocked, setAiLocked] = useState(false);
  const [summary, setSummary] = useState("");
  const [summarizing, setSummarizing] = useState(false);
  const [prioritizing, setPrioritizing] = useState(false);
  const [aiSearching, setAiSearching] = useState(false);
  const [priorityOn, setPriorityOn] = useState(false);
  const [priorityCategories, setPriorityCategories] = useState<Record<string, string>>({});
  // Snapshot of the pre-priority list so leaving priority view restores it.
  const [baseMessages, setBaseMessages] = useState<MailMessage[] | null>(null);

  // load AI capability once; AI summary/draft only show when a provider is configured
  useEffect(() => {
    aiStatus()
      .then((s) => {
        setAiEnabled(s.enabled);
        setAiLocked(false);
      })
      .catch((e) => {
        setAiEnabled(false);
        setAiLocked(e instanceof ApiError && e.status === 404);
      });
  }, []);

  // Effective per-feature AI flags: backend availability AND the user toggles.
  const ai = useMemo(
    () => ({
      summary: aiEnabled && prefsAiEnabled && prefsAi.summary,
      draft: aiEnabled && prefsAiEnabled && prefsAi.draft,
      priority: aiEnabled && prefsAiEnabled && prefsAi.priority,
      search: aiEnabled && prefsAiEnabled && prefsAi.search,
    }),
    [aiEnabled, prefsAiEnabled, prefsAi],
  );

  // summarize produces the reading-pane summary. The optional threadText lets
  // the conversation view summarize the whole thread (member bodies joined)
  // instead of just the opened member; omitting it summarizes the detail.
  async function summarize(threadText?: string) {
    const body = threadText ?? detail?.text_body ?? detail?.html_body ?? "";
    if (!body) return;
    setSummarizing(true);
    setSummary("");
    try {
      const res = await aiSummarize(body);
      setSummary(res.summary);
    } catch (e) {
      setError(e instanceof Error ? e.message : "summarize failed");
    } finally {
      setSummarizing(false);
    }
  }

  async function togglePriority() {
    if (priorityOn) {
      if (baseMessages) setMessages(baseMessages);
      setPriorityOn(false);
      setPriorityCategories({});
      setBaseMessages(null);
      return;
    }
    if (!ai.priority || messages.length === 0) return;
    setPrioritizing(true);
    setError("");
    try {
      const items = messages.map((m) => ({
        uid: m.uid,
        subject: m.subject,
        from: m.from[0]?.email || "",
      }));
      const { scores, categories } = await aiPrioritize(items);
      setBaseMessages(messages);
      setMessages(
        [...messages].sort(
          (a, b) => (scores[String(b.uid)] ?? 0) - (scores[String(a.uid)] ?? 0),
        ),
      );
      setPriorityCategories(categories ?? {});
      setPriorityOn(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "prioritize failed");
    } finally {
      setPrioritizing(false);
    }
  }

  async function doAiSearch(q?: string) {
    const searchQuery = (q ?? query).trim();
    if (!searchQuery || !ai.search) return;
    // Invalidate in-flight folder loads / keyword searches and take the
    // sequence ourselves (runSearchWithSpec pattern); a folder switch after
    // this point bumps the counters and our late result is dropped.
    const seq = ++searchSeqRef.current;
    loadSeq.current++;
    setAiSearching(true);
    setError("");
    try {
      const res = await aiSearch(searchQuery);
      if (seq !== searchSeqRef.current) return; // superseded
      setSearching(true);
      setSelected(null);
      setDetail(null);
      setCursor(0);
      setMessages(res.messages);
    } catch (e) {
      setError(e instanceof Error ? e.message : "ai search failed");
    } finally {
      setAiSearching(false);
    }
  }

  return {
    summary, setSummary,
    summarizing,
    prioritizing,
    aiSearching,
    priorityOn, setPriorityOn,
    priorityCategories, setPriorityCategories,
    baseMessages, setBaseMessages,
    ai,
    aiLocked,
    summarize, togglePriority, doAiSearch,
  };
}
