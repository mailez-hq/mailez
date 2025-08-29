"use client";

import { useEffect, useRef, useState, type Dispatch, type RefObject, type SetStateAction } from "react";

import { SAVED_SEARCH_KEY, type SavedSearch } from "@/components/mailbox/mail-utils";
import { mailSearchSpec, mailUnseen, type MailMessage, type MailSearchSpec } from "@/lib/api";
import { buildSearchSpec } from "@/lib/mail-search";

/**
 * Search & virtual-view cluster: the keyword box, structured search specs,
 * label filters, saved (virtual) searches and the search-aware refresh.
 * Owns the search-related state; the list/reader state it clears stays in
 * the store and is passed in as setters.
 */
export function useMailSearch({
  folder,
  pathId,
  router,
  loadMessages,
  loadSeq,
  setMessages,
  setSelected,
  setDetail,
  setCursor,
  setUnseen,
  setError,
  setRefreshing,
}: {
  folder: string;
  /** Non-null while the URL points at an opened message (/mail/<folder>/<id>). */
  pathId: string | null;
  router: { replace: (url: string, options?: { scroll?: boolean }) => void };
  loadMessages: (folder: string, page?: number, silent?: boolean) => Promise<void>;
  /** The store's folder-load sequence guard; searches invalidate in-flight folder loads. */
  loadSeq: RefObject<number>;
  setMessages: Dispatch<SetStateAction<MailMessage[]>>;
  setSelected: Dispatch<SetStateAction<MailMessage | null>>;
  setDetail: Dispatch<SetStateAction<MailMessage | null>>;
  setCursor: Dispatch<SetStateAction<number>>;
  setUnseen: Dispatch<SetStateAction<Record<string, number>>>;
  setError: Dispatch<SetStateAction<string>>;
  setRefreshing: Dispatch<SetStateAction<boolean>>;
}) {
  const [query, setQuery] = useState("");
  const [searching, setSearching] = useState(false);
  const [searchSpec, setSearchSpec] = useState<MailSearchSpec | null>(null);
  const [activeView, setActiveView] = useState("all");
  const [activeLabel, setActiveLabel] = useState("");
  const [savedSearches, setSavedSearches] = useState<SavedSearch[]>([]);
  const [searchAll, setSearchAll] = useState(false);
  // Same monotonic-sequence guard as folder loads (independent counter):
  // rapid saved-search / label / typing transitions fire overlapping
  // /mail/search requests and only the newest may land.
  const searchSeq = useRef(0);
  const lastSearchRef = useRef("");

  async function doSearch(e?: React.FormEvent | string) {
    if (typeof e !== "string") e?.preventDefault();
    const q = (typeof e === "string" ? e : query).trim();
    const spec = buildSearchSpec(q, searchSpec);
    if (!spec) {
      // An empty submission with no builder conditions means "show the plain
      // folder": clear the search state and reload, otherwise stale filtered
      // results would stay on screen with no indicator.
      clearSearch();
      return;
    }
    // A real search replaces any active label filter.
    setActiveLabel("");
    await runSearchWithSpec(spec);
  }

  // runSearchWithSpec executes a structured search (visual builder or merged
  // keywords) straight against /mail/search — no syntax-string round-trip.
  async function runSearchWithSpec(spec: MailSearchSpec) {
    // Monotonic seq: a slow earlier search must not overwrite a newer one
    // (rapid saved-search / label / typing transitions fire overlapping
    // requests).
    const seq = ++searchSeq.current;
    // Invalidate any in-flight folder load: without this, a slow
    // loadMessages response landing after the search result would overwrite
    // the filtered list with the full folder (the two seq guards are
    // independent).
    loadSeq.current++;
    setSearching(true);
    setSelected(null);
    setDetail(null);
    setCursor(0);
    // Drop any opened-message segment from the URL: the reading pane is being
    // cleared with the search, and leaving a stale /mail/<folder>/<id> makes a
    // later click on the same row a no-op (pathId never changes, so the
    // pathId effect never re-runs).
    if (pathId) {
      router.replace(`/mail/${encodeURIComponent(folder)}`, { scroll: false });
    }
    try {
      const results = await mailSearchSpec(searchAll ? "all" : folder, spec);
      if (seq !== searchSeq.current) return;
      setMessages(results);
      lastSearchRef.current = spec.text?.join(" ") ?? "";
    } catch (err) {
      if (seq !== searchSeq.current) return;
      setError(err instanceof Error ? err.message : "search failed");
    }
  }

  // applySearchSpec is the visual builder's submit: it replaces the keyword
  // box with the structured conditions and searches immediately.
  function applySearchSpec(spec: MailSearchSpec) {
    setSearchSpec(spec);
    setQuery("");
    setActiveView("all");
    setActiveLabel("");
    runSearchWithSpec(spec);
  }

  // Instant search: debounce manual typing so results appear live (except when
  // a virtual view / label filter already ran its own explicit search).
  useEffect(() => {
    const q = query.trim();
    if (q === "" || activeView !== "all" || activeLabel !== "" || q === lastSearchRef.current) return;
    const id = setTimeout(() => doSearch(q), 400);
    return () => clearTimeout(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- doSearch reads exactly these inputs; its unstable identity would re-arm the debounce every render
  }, [query, activeView, activeLabel]);

  function clearSearch() {
    setSearching(false);
    setQuery("");
    setSearchSpec(null);
    setActiveView("all");
    setActiveLabel("");
    lastSearchRef.current = "";
    setCursor(0);
    if (pathId) {
      router.replace(`/mail/${encodeURIComponent(folder)}`, { scroll: false });
    }
    loadMessages(folder);
  }

  // refreshMail reloads the current view without blanking the list: re-runs an
  // active search, otherwise refetches the folder plus unseen counts.
  async function refreshMail() {
    setRefreshing(true);
    try {
      if (searching) {
        if (activeLabel) {
          // Re-run the label filter instead of dropping back to the folder.
          await runSearchWithSpec({ labels: [activeLabel] });
        } else {
          await doSearch(query);
        }
      } else {
        await Promise.all([loadMessages(folder, 0, true), mailUnseen().then(setUnseen)]);
      }
    } finally {
      setRefreshing(false);
    }
  }

  // Label filter (clicking a tag in the sidebar).
  function selectLabel(label: string) {
    setActiveView("all");
    setActiveLabel(label);
    // Drop any stale builder conditions so they can't leak into the label
    // view or into a later search box submission.
    setSearchSpec(null);
    if (label) {
      // Run the structured search directly; keep the keyword box empty
      // instead of echoing the raw label: syntax into it.
      setQuery("");
      runSearchWithSpec({ labels: [label] });
    } else {
      setQuery("");
      clearSearch();
    }
  }

  // Saved searches (virtual folders): keyword queries and structured specs.
  useEffect(() => {
    try {
      const raw = localStorage.getItem(SAVED_SEARCH_KEY);
      if (!raw) return;
      const parsed: unknown = JSON.parse(raw);
      if (!Array.isArray(parsed)) return;
      // Legacy entries are plain query strings; migrate them to typed items.
      const migrated: SavedSearch[] = parsed.map((item, i) => {
        if (typeof item === "string") {
          return { id: i + 1, kind: "query", name: item, query: item };
        }
        const s = item as {
          id?: number;
          kind?: string;          name?: string;
          query?: string;
          spec?: MailSearchSpec;
        };
        const id = s.id ?? i + 1;
        const name = s.name ?? s.query ?? "";
        if (s.kind === "spec" && s.spec) {
          return { id, kind: "spec", name, spec: s.spec };
        }
        return { id, kind: "query", name, query: s.query ?? "" };
      });
      // Hydration-safe storage read: the server render must show the empty
      // default (a lazy initializer would diverge the SSR markup), so the
      // persisted list lands right after mount — an intentional post-hydration
      // write, not a cascading render.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setSavedSearches(migrated);
    } catch {
      // storage unavailable
    }
  }, []);

  function persistSearches(next: SavedSearch[]) {
    setSavedSearches(next);
    try {
      localStorage.setItem(SAVED_SEARCH_KEY, JSON.stringify(next));
    } catch {
      // storage unavailable
    }
  }

  function saveCurrentSearch() {
    const q = query.trim();
    if (!q || savedSearches.some((s) => s.kind === "query" && s.query === q)) return;
    persistSearches([...savedSearches, { id: Date.now(), kind: "query", name: q, query: q }]);
  }

  // saveSearchSpec persists a structured condition set from the search builder
  // as a named quick entry in the sidebar.
  function saveSearchSpec(name: string, spec: MailSearchSpec) {
    const n = name.trim();
    if (!n || savedSearches.some((s) => s.kind === "spec" && s.name === n)) return;
    persistSearches([...savedSearches, { id: Date.now(), kind: "spec", name: n, spec }]);
  }

  function removeSavedSearch(id: number) {
    persistSearches(savedSearches.filter((s) => s.id !== id));
  }

  function runSavedSearch(item: SavedSearch) {
    if (item.kind === "spec") {
      applySearchSpec(item.spec);
      return;
    }
    // A keyword saved search replaces, not merges with, any stale builder
    // conditions from an earlier search. Build the spec explicitly with no
    // prior conditions so the synchronous call below can't capture the old
    // searchSpec and fire a duplicate, stale-merged request.
    setSearchSpec(null);
    setQuery(item.query);
    const spec = buildSearchSpec(item.query, null);
    if (spec) {
      setActiveLabel("");
      runSearchWithSpec(spec);
    }
  }

  return {
    query, setQuery,
    searching, setSearching,
    searchSpec, setSearchSpec,
    activeView, setActiveView,
    activeLabel, setActiveLabel,
    savedSearches, searchAll, setSearchAll,
    // Exposed so the store can clear it on folder switches.
    lastSearchRef,
    doSearch, runSearchWithSpec, applySearchSpec,
    clearSearch, refreshMail, selectLabel,
    saveCurrentSearch, saveSearchSpec, removeSavedSearch, runSavedSearch,
  };
}
