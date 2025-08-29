"use client";

import type { Dispatch, SetStateAction } from "react";

import {
  mailFlag,
  mailMove,
  mailThread,
  mailUnsubscribe,
  meProfile,
  updateMeSettings,
  type MailMessage,
} from "@/lib/api";
import { PIN_FLAG, MUTE_FLAG, isPinned, isMuted } from "@/components/mailbox/mail-utils";
import type { Translate } from "./use-folder-mgmt";
import { detailCache } from "./view-caches";

/**
 * Message-action cluster: the move family (move/archive/spam/trash + bulk
 * variants with undo), flag operations (seen/star/pin/mute), opening a
 * message (URL navigation + optimistic seen), and sender actions (report
 * not-spam, list-unsubscribe). List/detail state stays in their owning
 * hooks and is passed in as setters.
 */
export function useMessageActions({
  folder,
  messages,
  searching,
  selected,
  detail,
  selectedUids,
  spamFolder,
  setSelected,
  setDetail,
  setMessages,
  setSelectedUids,
  setError,
  showToast,
  t,
  router,
  refreshUnseen,
  refreshMail,
  loadMessages,
  openCompose,
}: {
  folder: string;
  messages: MailMessage[];
  searching: boolean;
  selected: MailMessage | null;
  detail: MailMessage | null;
  selectedUids: Set<number>;
  spamFolder: string;
  setSelected: Dispatch<SetStateAction<MailMessage | null>>;
  setDetail: Dispatch<SetStateAction<MailMessage | null>>;
  setMessages: Dispatch<SetStateAction<MailMessage[]>>;
  setSelectedUids: Dispatch<SetStateAction<Set<number>>>;
  setError: Dispatch<SetStateAction<string>>;
  showToast: (label: string, onUndo?: () => void, duration?: number) => void;
  t: Translate;
  router: { push: (href: string) => void };
  refreshUnseen: () => void;
  refreshMail: () => void | Promise<void>;
  loadMessages: (folder: string, page?: number, silent?: boolean) => Promise<void>;
  openCompose: (toAddr?: string, subj?: string, html?: string, text?: string, focus?: "to" | "editor", replyTarget?: boolean) => void;
}) {
  // groupUidsByFolder resolves each uid to its owning folder: cross-folder
  // search results (searchAll / AI search) span folders and IMAP uids are
  // only unique within one folder — acting on them with the open folder's
  // name would hit an unrelated message that happens to share the uid.
  function groupUidsByFolder(uids: number[]): Map<string, number[]> {
    const byUid = new Map(messages.map((m) => [m.uid, m.folder || folder]));
    const groups = new Map<string, number[]>();
    for (const u of uids) {
      const f = byUid.get(u) || folder;
      const list = groups.get(f);
      if (list) list.push(u);
      else groups.set(f, [u]);
    }
    return groups;
  }

  // moveTo moves messages to a folder and offers an undo that moves them back.
  async function moveTo(uids: number[], destination: string, successLabel: string) {
    setError("");
    try {
      const groups = groupUidsByFolder(uids);
      await Promise.all([...groups].map(([src, us]) => mailMove(src, us, destination)));
      refreshUnseen();
      const uidSet = new Set(uids);
      setMessages((ms) => ms.filter((x) => !uidSet.has(x.uid)));
      setSelectedUids((prev) => {
        const next = new Set(prev);
        uids.forEach((u) => next.delete(u));
        return next;
      });
      if (selected && uidSet.has(selected.uid)) {
        setSelected(null);
        setDetail(null);
      }
      showToast(successLabel, () => {
        // Undo returns each batch to the folder it came from.
        Promise.all([...groups].map(([src, us]) => mailMove(destination, us, src)))
          .catch(() => {})
          .finally(() => loadMessages(folder));
      });
    } catch (e) {
      setError(e instanceof Error ? e.message : "move failed");
    }
  }

  function archiveMessage(m: MailMessage) {
    return moveTo([m.uid], "Archive", t("toastArchived"));
  }

  function spamMessage(m: MailMessage) {
    return moveTo([m.uid], spamFolder, t("toastSpam"));
  }

  // Report not-spam: whitelist the sender and move the message back to Inbox.
  async function reportNotSpam(m: MailMessage) {
    const sender = m.from[0]?.email;
    setError("");
    try {
      if (sender) {
        const profile = await meProfile();
        const current = (profile.whitelist || "").split(",").map((s) => s.trim()).filter(Boolean);
        if (!current.includes(sender)) current.push(sender);
        await updateMeSettings({ whitelist: current.join(", ") });
      }
      await moveTo([m.uid], "Inbox", t("toastNotSpam"));
    } catch (e) {
      setError(e instanceof Error ? e.message : "not spam failed");
    }
  }

  // unsubscribeAction follows the sender's List-Unsubscribe header: mailto
  // opens a pre-filled compose, https is triggered server-side.
  async function unsubscribeAction(m: MailMessage) {
    const url = m.unsubscribe_url;
    if (!url) return;
    if (url.startsWith("mailto:")) {
      const addr = url.slice("mailto:".length).split("?")[0];
      openCompose(addr, "Unsubscribe", "", "", "editor");
      return;
    }
    setError("");
    try {
      await mailUnsubscribe(url, !!m.unsubscribe_post);
      showToast(t("toastUnsubscribed"));
    } catch (e) {
      setError(e instanceof Error ? e.message : t("toastUnsubscribeFailed"));
    }
  }

  function openMessage(m: MailMessage, srcFolder?: string) {
    // Cross-folder rows carry their owning folder; the open folder is only
    // the fallback for plain folder browsing.
    const f = srcFolder ?? m.folder ?? folder;
    if (!m.flags.includes("\\Seen")) {
      detailCache.delete(`${f}\x00${m.id || m.uid}`);
      mailFlag(f, m.uid, "\\Seen", true).then(refreshUnseen).catch(() => {});
      m.flags.push("\\Seen");
      setMessages((ms) => ms.map((x) => (x.uid === m.uid ? applySeenState(x, true) : x)));
      // A multi-member thread's unread dot is a server-side aggregate that
      // cannot be derived from the opened message alone: reload the folder so
      // reading the last unread member clears the row's unread indicator.
      // Skipped while searching (results are not the folder list).
      const row = messages.find((x) => x.uid === m.uid);
      if (!searching && row?.thread_unread && row.thread_count && row.thread_count > 1) {
        void loadMessages(f, 0, true);
      }
    }
    // Navigate to the message route; MailView follows the /mail/[folder]/[id]
    // URL (pathId effect) to load and show the reading pane. The stable id
    // keeps every view routable, shareable and survives mailbox moves. The
    // folder is URL-encoded so nested names (Parent/Child) stay one segment.
    router.push(`/mail/${encodeURIComponent(f)}/${m.id || m.uid}`);
  }

  function removeMessage(m: MailMessage) {
    return moveTo([m.uid], "Trash", t("toastDeleted"));
  }

  async function bulkDelete() {
    const ids = [...selectedUids];
    await moveTo(ids, "Trash", t("toastDeleted"));
  }

  async function bulkArchive() {
    await moveTo([...selectedUids], "Archive", t("toastArchived"));
  }

  async function bulkSpam() {
    await moveTo([...selectedUids], spamFolder, t("toastSpam"));
  }

  function moveSelectedTo(destination: string) {
    return moveTo([...selectedUids], destination, t("toastMoved"));
  }

  function moveDetailTo(destination: string) {
    if (detail) return moveTo([detail.uid], destination, t("toastMoved"));
  }

  async function bulkFlag(flag: string, value: boolean) {
    const ids = [...selectedUids];
    setError("");
    try {
      const groups = groupUidsByFolder(ids);
      await Promise.all([...groups].map(([src, us]) => us.map((uid) => mailFlag(src, uid, flag, value))).flat());
      refreshUnseen();
      setMessages((ms) =>
        ms.map((m) =>
          selectedUids.has(m.uid)
            ? { ...m, flags: value ? [...m.flags, flag] : m.flags.filter((f) => f !== flag) }
            : m,
        ),
      );
      setSelectedUids(new Set());
    } catch (e) {
      setError(e instanceof Error ? e.message : "bulk flag failed");
    }
  }

  async function setSeen(m: MailMessage, value: boolean) {
    const f = m.folder || folder;
    detailCache.delete(`${f}\x00${m.id || m.uid}`);
    try {
      await mailFlag(f, m.uid, "\\Seen", value);
      refreshUnseen();
      const apply = (x: MailMessage) => (x.uid === m.uid ? applySeenState(x, value) : x);
      setMessages((ms) => ms.map(apply));
      setDetail((d) => (d ? apply(d) : d));
    } catch (e) {
      setError(e instanceof Error ? e.message : "flag failed");
    }
  }

  async function toggleStar(m: MailMessage) {
    const starred = !m.flags.includes("\\Flagged");
    const f = m.folder || folder;
    detailCache.delete(`${f}\x00${m.id || m.uid}`);
    try {
      await mailFlag(f, m.uid, "\\Flagged", starred);
      setMessages((ms) =>
        ms.map((x) =>
          x.uid === m.uid
            ? {
                ...x,
                flags: starred
                  ? [...new Set([...x.flags, "\\Flagged"])]
                  : x.flags.filter((f) => f !== "\\Flagged"),
              }
            : x,
        ),
      );
      if (detail?.uid === m.uid) {
        setDetail((d) =>
          d
            ? {
                ...d,
                flags: starred
                  ? [...new Set([...d.flags, "\\Flagged"])]
                  : d.flags.filter((f) => f !== "\\Flagged"),
              }
            : d,
        );
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "star failed");
    }
  }

  // Pinned messages stay at the top of the list via the $Pin keyword.
  async function togglePin(m: MailMessage) {
    const pinned = !isPinned(m);
    const f = m.folder || folder;
    try {
      await mailFlag(f, m.uid, PIN_FLAG, pinned);
      setMessages((ms) =>
        ms.map((x) =>
          x.uid === m.uid
            ? {
                ...x,
                flags: pinned
                  ? [...new Set([...x.flags, PIN_FLAG])]
                  : x.flags.filter((f) => f !== PIN_FLAG),
              }
            : x,
        ),
      );
      if (detail?.uid === m.uid) {
        setDetail((d) =>
          d
            ? {
                ...d,
                flags: pinned
                  ? [...new Set([...d.flags, PIN_FLAG])]
                  : d.flags.filter((f) => f !== PIN_FLAG),
              }
            : d,
        );
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "pin failed");
    }
  }

  async function toggleRead(m: MailMessage) {
    const value = !m.flags.includes("\\Seen");
    try {
      await mailFlag(m.folder || folder, m.uid, "\\Seen", value);
      const apply = (x: MailMessage) => (x.uid === m.uid ? applySeenState(x, value) : x);
      setMessages((ms) => ms.map(apply));
      setDetail((d) => (d && d.uid === m.uid ? apply(d) : d));
    } catch (e) {
      setError(e instanceof Error ? e.message : "mark read failed");
    }
  }

  // toggleMute mutes/unmutes the current conversation: muting archives every
  // message of the thread and marks them $Muted (the sidebar badge shows the
  // state; future replies need a server rule to skip the inbox automatically).
  async function toggleMute(m: MailMessage) {
    if (!m.thread_id) {
      // Single message with no thread metadata: mute just this message.
      const muted = isMuted(m);
      try {
        await mailFlag(m.folder || folder, m.uid, MUTE_FLAG, !muted);
        if (!muted) await mailMove(m.folder || folder, [m.uid], "Archive");
        refreshMail();
        showToast(t(muted ? "toastUnmuted" : "toastMuted"));
      } catch (e) {
        setError(e instanceof Error ? e.message : "mute failed");
      }
      return;
    }
    try {
      const th = await mailThread(m.folder || folder, m.thread_id);
      const msgs = th.messages || [];
      const muted = msgs.some((x) => isMuted(x));
      const targets = msgs.length ? msgs : [m];
      const uids = targets.map((x) => x.uid);
      const src = m.folder || folder;
      for (const x of targets) {
        await mailFlag(src, x.uid, MUTE_FLAG, !muted);
      }
      if (!muted && uids.length) {
        await mailMove(src, uids, "Archive");
      }
      refreshMail();
      showToast(t(muted ? "toastUnmuted" : "toastMuted"));
    } catch (e) {
      setError(e instanceof Error ? e.message : "mute failed");
    }
  }

  return {
    moveTo, moveSelectedTo, moveDetailTo,
    archiveMessage, spamMessage, removeMessage,
    bulkDelete, bulkArchive, bulkSpam, bulkFlag,
    setSeen, toggleRead, toggleStar, togglePin, toggleMute,
    openMessage, reportNotSpam, unsubscribeAction,
  };
}

// applySeenState returns a list row with the \Seen flag applied and, for
// conversation rows, the thread aggregate resolved when it can be computed
// locally: a single-message thread turns read/unread with the row. Rows of
// multi-member threads keep their server-side aggregate until the folder
// is reloaded (see openMessage).
export function applySeenState(x: MailMessage, seen: boolean): MailMessage {
  const flags = seen
    ? x.flags.includes("\\Seen")
      ? x.flags
      : [...x.flags, "\\Seen"]
    : x.flags.filter((f) => f !== "\\Seen");
  let thread_unread = x.thread_unread;
  if (x.thread_unread !== undefined) {
    if (!seen) thread_unread = true;
    else if (!x.thread_count || x.thread_count <= 1) thread_unread = false;
  }
  return {...x, flags, thread_unread};
}
