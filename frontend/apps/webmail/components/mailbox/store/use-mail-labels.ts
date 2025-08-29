"use client";

import { useEffect, useMemo, useState, type Dispatch, type SetStateAction } from "react";

import { mailFlag, mailLabelDelete, mailLabelRename, mailLabelSave, mailLabels, type MailLabel, type MailMessage } from "@/lib/api";
import { isUserLabel } from "@/components/mailbox/mail-utils";

/**
 * Label cluster: server-side label definitions (name + color), the user
 * keywords observed on loaded messages, their union (knownLabels) and colors,
 * and the label manager dialog state. Label changes are reflected optimistically
 * onto the message list and the open detail.
 */
export function useMailLabels({
  messages,
  folder,
  activeLabel,
  selectLabel,
  selectedUids,
  setSelectedUids,
  setError,
  setMessages,
  setDetail,
  showToast,
  refreshMail,
  t,
}: {
  messages: MailMessage[];
  folder: string;
  /** The currently active label filter (owned by useMailSearch). */
  activeLabel: string;
  /** Switching/renaming/deleting the active label must re-run the filter. */
  selectLabel: (label: string) => void;
  selectedUids: Set<number>;
  setSelectedUids: Dispatch<SetStateAction<Set<number>>>;
  setError: Dispatch<SetStateAction<string>>;
  setMessages: Dispatch<SetStateAction<MailMessage[]>>;
  setDetail: Dispatch<SetStateAction<MailMessage | null>>;
  showToast: (label: string, onUndo?: () => void, duration?: number) => void;
  refreshMail: () => void | Promise<void>;
  t: (key: string, values?: Record<string, string | number | Date>) => string;
}) {
  const [labelDefs, setLabelDefs] = useState<MailLabel[]>([]);
  const [flagLabels, setFlagLabels] = useState<string[]>([]);
  const [labelManagerOpen, setLabelManagerOpen] = useState(false);

  // Collect user labels (custom IMAP keywords) from loaded messages so the
  // sidebar and reader can offer them for quick tagging/filtering; the list
  // is merged with the server-side definitions in knownLabels below. The
  // merge runs during render as a React-endorsed state adjustment on message
  // list changes.
  const [prevFlagMsgs, setPrevFlagMsgs] = useState(messages);
  if (messages !== prevFlagMsgs) {
    setPrevFlagMsgs(messages);
    const merged = [...flagLabels];
    for (const m of messages) {
      for (const f of m.flags) {
        if (isUserLabel(f) && !merged.includes(f)) merged.push(f);
      }
    }
    setFlagLabels(merged);
  }

  const knownLabels = useMemo(() => {
    const set = new Set<string>(labelDefs.map((d) => d.name));
    flagLabels.forEach((f) => set.add(f));
    return [...set];
  }, [labelDefs, flagLabels]);

  const labelColors = useMemo(() => {
    const out: Record<string, string> = {};
    labelDefs.forEach((d) => {
      if (d.color) out[d.name] = d.color;
    });
    return out;
  }, [labelDefs]);

  // Load label definitions once so the sidebar lists labels even when no
  // currently loaded message carries the keyword.
  useEffect(() => {
    mailLabels().then(setLabelDefs).catch(() => {});
  }, []);

  async function toggleLabel(m: MailMessage, label: string) {
    const has = m.flags.includes(label);
    setError("");
    // Applying a brand-new tag creates its definition so it stays listed and
    // gets a stable color; the palette fallback covers races.
    if (!has && !labelDefs.some((d) => d.name === label)) {
      mailLabelSave(label, "")
        .then((saved) => setLabelDefs((ds) => [...ds, saved]))
        .catch(() => {});
    }
    try {
      await mailFlag(folder, m.uid, label, !has);
      setMessages((ms) =>
        ms.map((x) =>
          x.uid === m.uid
            ? { ...x, flags: has ? x.flags.filter((f) => f !== label) : [...x.flags, label] }
            : x,
        ),
      );
      setDetail((d) =>
        d && d.uid === m.uid
          ? { ...d, flags: has ? d.flags.filter((f) => f !== label) : [...d.flags, label] }
          : d,
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "label failed");
    }
  }

  // saveLabel creates/updates a label definition (name + color).
  async function saveLabel(name: string, color: string) {
    const saved = await mailLabelSave(name, color);
    setLabelDefs((ds) => {
      const i = ds.findIndex((d) => d.name === name);
      if (i < 0) return [...ds, saved];
      const copy = [...ds];
      copy[i] = saved;
      return copy;
    });
  }

  // renameLabel renames the keyword on every message (server-side) and
  // updates the local definitions and flags optimistically.
  async function renameLabel(from: string, to: string) {
    await mailLabelRename(from, to);
    setLabelDefs((ds) => ds.map((d) => (d.name === from ? { ...d, name: to } : d)));
    const swap = (flags: string[]) => flags.map((f) => (f === from ? to : f));
    setMessages((ms) => ms.map((x) => (x.flags.includes(from) ? { ...x, flags: swap(x.flags) } : x)));
    setDetail((d) => (d && d.flags.includes(from) ? { ...d, flags: swap(d.flags) } : d));
    setFlagLabels((fs) => fs.map((f) => (f === from ? to : f)));
    if (activeLabel === from) selectLabel(to);
  }

  // deleteLabel strips the keyword from every message (server-side) and
  // removes the definition plus the local flag occurrences.
  async function deleteLabel(name: string) {
    await mailLabelDelete(name);
    setLabelDefs((ds) => ds.filter((d) => d.name !== name));
    setFlagLabels((fs) => fs.filter((f) => f !== name));
    setMessages((ms) => ms.map((x) => ({ ...x, flags: x.flags.filter((f) => f !== name) })));
    setDetail((d) => (d ? { ...d, flags: d.flags.filter((f) => f !== name) } : d));
    if (activeLabel === name) selectLabel("");
  }

  // bulkLabel applies a label to every selected message. A brand-new name is
  // persisted as a label definition first so the sidebar keeps it.
  async function bulkLabel(label: string) {
    if (selectedUids.size === 0 || !label.trim()) return;
    setError("");
    try {
      const name = label.trim();
      if (!labelDefs.some((d) => d.name === name)) {
        const saved = await mailLabelSave(name, "");
        setLabelDefs((ds) => [...ds, saved]);
      }
      for (const uid of selectedUids) {
        await mailFlag(folder, uid, name, true);
      }
      setSelectedUids(new Set());
      showToast(t("toastLabelApplied"));
      refreshMail();
    } catch (e) {
      setError(e instanceof Error ? e.message : "label failed");
    }
  }

  return {
    labelDefs, setLabelDefs,
    flagLabels, labelManagerOpen, setLabelManagerOpen,
    knownLabels, labelColors,
    toggleLabel, saveLabel, renameLabel, deleteLabel, bulkLabel,
  };
}
