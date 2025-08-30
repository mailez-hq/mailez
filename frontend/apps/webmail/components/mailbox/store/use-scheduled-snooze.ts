"use client";

import { useCallback, useState, type Dispatch, type RefObject, type SetStateAction } from "react";

import { mailSnooze, mailSnoozed, mailScheduled, mailUndoSend, type MailMessage, type MailSearchSpec, type ScheduledSend, type SnoozedMessage } from "@/lib/api";
import type { Translate } from "./use-folder-mgmt";

/**
 * Time-shifted mail cluster: scheduled sends (the backend parks messages in
 * the outbox until send_at; the dialog lists and cancels them) and snoozed
 * messages (keyword-based; the backend resurfaces due ones lazily).
 */
export function useScheduledSnooze({
  folder,
  setMessages,
  setTotal,
  setSelected,
  setDetail,
  setCursor,
  setQuery,
  setSearching,
  setSearchSpec,
  setActiveView,
  setActiveLabel,
  setError,
  showToast,
  refreshMail,
  searchSeqRef,
  loadSeq,
  t,
}: {
  folder: string;
  setMessages: Dispatch<SetStateAction<MailMessage[]>>;
  setTotal: Dispatch<SetStateAction<number>>;
  setSelected: Dispatch<SetStateAction<MailMessage | null>>;
  setDetail: Dispatch<SetStateAction<MailMessage | null>>;
  setCursor: Dispatch<SetStateAction<number>>;
  setQuery: Dispatch<SetStateAction<string>>;
  setSearching: Dispatch<SetStateAction<boolean>>;
  setSearchSpec: Dispatch<SetStateAction<MailSearchSpec | null>>;
  setActiveView: Dispatch<SetStateAction<string>>;
  setActiveLabel: Dispatch<SetStateAction<string>>;
  setError: Dispatch<SetStateAction<string>>;
  showToast: (label: string, onUndo?: () => void, duration?: number) => void;
  refreshMail: () => void | Promise<void>;
  /** Same sequence guards the folder/search loaders use: opening the
   * snoozed view replaces the list, so any in-flight folder load or search
   * response must be invalidated, and vice versa. */
  searchSeqRef: RefObject<number>;
  loadSeq: RefObject<number>;
  t: Translate;
}) {
  const [scheduled, setScheduled] = useState<ScheduledSend[]>([]);
  const [scheduledOpen, setScheduledOpen] = useState(false);
  const [scheduledLoading, setScheduledLoading] = useState(false);
  const [snoozedMsgs, setSnoozedMsgs] = useState<SnoozedMessage[]>([]);

  const loadScheduled = useCallback(async () => {
    setScheduledLoading(true);
    try {
      setScheduled(await mailScheduled());
    } catch (e) {
      setError(e instanceof Error ? e.message : "load scheduled failed");
    } finally {
      setScheduledLoading(false);
    }
  }, [setError]);

  async function cancelScheduled(id: number) {
    setError("");
    try {
      await mailUndoSend(id);
      await loadScheduled();
      showToast(t("toastScheduledCancelled"));
    } catch (e) {
      setError(e instanceof Error ? e.message : "cancel scheduled failed");
    }
  }

  // Snooze hides a message until a given time; untilMs is 0 to wake it up.
  async function snoozeMessage(m: MailMessage, untilMs: number) {
    const f = m.folder || folder;
    try {
      await mailSnooze(f, m.uid, untilMs > 0 ? Math.floor(untilMs / 1000) : null);
      if (untilMs > 0) {
        // Hide from the current list; it now lives under the Snoozed view.
        setMessages((ms) => ms.filter((x) => x.uid !== m.uid));
        showToast(t("toastSnoozed"));
      } else {
        setSnoozedMsgs((ms) => ms.filter((x) => x.uid !== m.uid));
        refreshMail();
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "snooze failed");
    }
  }

  // Wake a snoozed message back into its folder.
  async function unsnooze(sm: SnoozedMessage) {
    const f = sm.folder || folder;
    try {
      await mailSnooze(f, sm.uid, null);
      setSnoozedMsgs((ms) => ms.filter((x) => x.uid !== sm.uid));
      refreshMail();
    } catch (e) {
      setError(e instanceof Error ? e.message : "unsnooze failed");
    }
  }

  // openSnoozed switches to the Snoozed virtual view, listing every message
  // parked by the user until a later time.
  async function openSnoozed() {
    // Invalidate any in-flight folder load / keyword search, exactly like
    // runSearchWithSpec does: their late responses must not clobber this
    // view, and this request must lose to anything the user starts next.
    const seq = ++searchSeqRef.current;
    loadSeq.current++;
    setActiveView("snoozed");
    setActiveLabel("");
    setSearchSpec(null);
    setQuery("");
    setSearching(false);
    setCursor(0);
    setSelected(null);
    setDetail(null);
    try {
      const list = await mailSnoozed();
      if (seq !== searchSeqRef.current) return; // superseded by a newer view
      setSnoozedMsgs(list);
      setMessages(list as unknown as MailMessage[]);
      setTotal(list.length);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load snoozed failed");
    }
  }

  return {
    scheduled, scheduledOpen, setScheduledOpen, scheduledLoading,
    loadScheduled, cancelScheduled,
    snoozedMsgs, snoozeMessage, unsnooze, openSnoozed,
  };
}
