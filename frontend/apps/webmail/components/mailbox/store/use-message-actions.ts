"use client";

import { useRef } from "react";
import type { Dispatch, SetStateAction } from "react";

import {
  mailDelete,
  mailFlag,
  mailMove,
  mailThread,
  mailUnsubscribe,
  meProfile,
  updateMeSettings,
  type MailMessage,
} from "@/lib/api";
import { PIN_FLAG, MUTE_FLAG, isPinned, isMuted, normalizeThread, selectionKey } from "@/components/mailbox/mail-utils";
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
  conversation,
  selected,
  detail,
  selectedUids,
  spamFolder,
  setSelected,
  setDetail,
  setMessages,
  setTotal,
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
  conversation: boolean;
  selected: MailMessage | null;
  detail: MailMessage | null;
  selectedUids: Set<string>;
  spamFolder: string;
  setSelected: Dispatch<SetStateAction<MailMessage | null>>;
  setDetail: Dispatch<SetStateAction<MailMessage | null>>;
  setMessages: Dispatch<SetStateAction<MailMessage[]>>;
  setTotal: Dispatch<SetStateAction<number>>;
  setSelectedUids: Dispatch<SetStateAction<Set<string>>>;
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
  // name would hit an unrelated message that happens to share the uid. The
  // fallback covers uids that are not in the current list (e.g. thread
  // members fetched for a conversation-wide action).
  function groupUidsByFolder(uids: number[], fallback?: string): Map<string, number[]> {
    const def = fallback ?? folder;
    const byUid = new Map(messages.map((m) => [m.uid, m.folder || folder]));
    const groups = new Map<string, number[]>();
    for (const u of uids) {
      const f = byUid.get(u) || def;
      const list = groups.get(f);
      if (list) list.push(u);
      else groups.set(f, [u]);
    }
    return groups;
  }

  // The folder at undo-click time may differ from the folder at move time;
  // restore into whatever the user is looking at now.
  const folderRef = useRef(folder);
  folderRef.current = folder;

  // moveTo moves messages to a folder and offers an undo that moves them back.
  async function moveTo(uids: number[], destination: string, successLabel: string, srcHint?: string) {
    return moveGroups(groupUidsByFolder(uids, srcHint), destination, successLabel);
  }

  async function moveGroups(groups: Map<string, number[]>, destination: string, successLabel: string) {
    const uids = [...groups.values()].flat();
    setError("");
    try {
      // Destination Trash from a source folder that is already Trash means
      // "delete for good": the server purges instead of moving.
      await Promise.all(
        [...groups].map(([src, us]) =>
          destination.toLowerCase() === "trash" && src.toLowerCase() === "trash"
            ? mailDelete(src, us)
            : mailMove(src, us, destination),
        ),
      );
      refreshUnseen();
      const uidSet = new Set(uids);
      setMessages((ms) => ms.filter((x) => !uidSet.has(x.uid)));
      // Keep `total` honest: only uids that actually left the open folder
      // shrink it, otherwise "Load more (N)" counts ghosts.
      const here = groups.get(folder)?.length ?? 0;
      if (here > 0) setTotal((n) => Math.max(0, n - here));
      // The optimistic filter above can be overwritten by a list reload that
      // was already in flight BEFORE the move landed (e.g. triggered by a
      // realtime arrival): its response still contains the moved rows, so
      // they pop right back. A silent reload issued NOW starts after the
      // server confirmed the move, so its snapshot is guaranteed post-move.
      void loadMessages(folderRef.current, 0, true);
      setSelectedUids((prev) => {
        const next = new Set(prev);
        // Keys come from the same folder-qualified rows as the groups.
        for (const [src, us] of groups) us.forEach((u) => next.delete(`${src}/${u}`));
        return next;
      });
      // Check both: a path may set detail without selected (thread detail).
      if ((selected && uidSet.has(selected.uid)) || (detail && uidSet.has(detail.uid))) {
        setSelected(null);
        setDetail(null);
        // The detail view is URL-driven: leaving the dead uid in the URL
        // turns the next reload into the "message gone" error path.
        router.push(`/mail/${encodeURIComponent(folder)}`);
      }
      showToast(successLabel, () => {
        // Undo returns each batch to the folder it came from.
        Promise.all([...groups].map(([src, us]) => mailMove(destination, us, src)))
          .catch(() => {})
          .finally(() => {
            loadMessages(folderRef.current);
            refreshUnseen();
          });
      });
    } catch (e) {
      setError(e instanceof Error ? e.message : "move failed");
    }
  }

  // A conversation row (and the reading pane it opens) stands for its whole
  // thread, exactly like openMessage's seen logic and toggleMute already
  // treat it: row/detail move actions must cover every member, not just the
  // representative — deleting only the visible member left the conversation
  // sitting in the list with a shrunken count, looking like delete silently
  // failed. Flat mode (conversation off) keeps strictly per-message actions.
  async function actionTarget(m: MailMessage): Promise<{ uids: number[]; src: string }> {
    const src = m.folder || folder;
    if (!conversation || !m.thread_id || !m.thread_count || m.thread_count <= 1) {
      return { uids: [m.uid], src };
    }
    try {
      const th = await mailThread(src, m.thread_id);
      const members = normalizeThread(th ?? { thread_id: m.thread_id, messages: [] }).messages;
      const uids = members.map((x) => x.uid);
      return { uids: uids.length ? uids : [m.uid], src };
    } catch {
      // Listing the thread failed; act on the visible message only.
      return { uids: [m.uid], src };
    }
  }

  async function archiveMessage(m: MailMessage) {
    const { uids, src } = await actionTarget(m);
    return moveTo(uids, "Archive", t("toastArchived"), src);
  }

  async function spamMessage(m: MailMessage) {
    const { uids, src } = await actionTarget(m);
    return moveTo(uids, spamFolder, t("toastSpam"), src);
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

  // Bulk release from the quarantine (junk) folder: whitelist every selected
  // sender in one settings update, then move all of them back to Inbox.
  async function reportNotSpamBulk() {
    const rows = selectedRows();
    const senders = [...new Set(
      rows.map((m) => m.from[0]?.email).filter((x): x is string => Boolean(x)),
    )];
    setError("");
    try {
      if (senders.length > 0) {
        const profile = await meProfile();
        const current = (profile.whitelist || "").split(",").map((s) => s.trim()).filter(Boolean);
        const merged = [...current];
        for (const s of senders) if (!merged.includes(s)) merged.push(s);
        if (merged.length !== current.length) {
          await updateMeSettings({ whitelist: merged.join(", ") });
        }
      }
      await moveGroups(groupRowsByFolder(rows), "Inbox", t("toastNotSpam"));
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
    // A conversation row stands for its whole thread: the row's
    // representative member can already be read while older members are
    // not, so "open = read" must cover the thread, not just the click
    // target — otherwise the row's unread dot and the folder's unseen
    // count never clear no matter how often the thread is opened.
    const threadId =
      m.thread_unread && m.thread_count && m.thread_count > 1 ? m.thread_id : undefined;
    if (!m.flags.includes("\\Seen") || threadId) {
      detailCache.delete(`${f}\x00${m.id || m.uid}`);
      if (!m.flags.includes("\\Seen")) {
        mailFlag(f, m.uid, "\\Seen", true).then(refreshUnseen).catch(() => {});
        m.flags.push("\\Seen");
        setMessages((ms) => ms.map((x) => (x.uid === m.uid ? applySeenState(x, true) : x)));
      }
      if (threadId) {
        void (async () => {
          try {
            const th = await mailThread(f, threadId);
            const members = normalizeThread(th ?? { thread_id: threadId, messages: [] }).messages;
            await Promise.all(
              members
                .filter((t) => !(t.flags ?? []).includes("\\Seen"))
                .map((t) => mailFlag(f, t.uid, "\\Seen", true).catch(() => {})),
            );
          } catch {
            // Listing the thread failed; keep the row's own state as-is.
          }
          refreshUnseen();
          // The thread's unread dot is a server-side aggregate; reload the
          // folder so clearing the last unread member clears the row too.
          // Skipped while searching (results are not the folder list).
          if (!searching) void loadMessages(f, 0, true);
        })();
      }
    }
    // Navigate to the message route; MailView follows the /mail/[folder]/[id]
    // URL (pathId effect) to load and show the reading pane. The stable id
    // keeps every view routable, shareable and survives mailbox moves. The
    // folder is URL-encoded so nested names (Parent/Child) stay one segment.
    router.push(`/mail/${encodeURIComponent(f)}/${m.id || m.uid}`);
  }

  async function removeMessage(m: MailMessage) {
    const { uids, src } = await actionTarget(m);
    return moveTo(uids, "Trash", t("toastDeleted"), src);
  }

  // The thread view's per-member "delete this message" action: strictly the
  // one message, even with the conversation view on — Gmail parity, where
  // the toolbar deletes the whole conversation but the member row offers a
  // single-message delete.
  async function removeSingleMessage(m: MailMessage) {
    return moveTo([m.uid], "Trash", t("toastDeleted"), m.folder || folder);
  }

  // Bulk actions act on rows the user can see: the selection resolves through
  // the live row list, so keys of rows that fell out of it never reach the
  // operation. Single-message actions keep their own resolution.
  function selectedRows(): MailMessage[] {
    return messages.filter((m) => selectedUids.has(selectionKey(m, folder)));
  }

  // groupRowsByFolder buckets rows into per-mailbox uid lists: IMAP uids
  // only identify a message within its own folder.
  function groupRowsByFolder(rows: MailMessage[]): Map<string, number[]> {
    const groups = new Map<string, number[]>();
    for (const m of rows) {
      const f = m.folder || folder;
      const list = groups.get(f);
      if (list) {
        if (!list.includes(m.uid)) list.push(m.uid);
      } else groups.set(f, [m.uid]);
    }
    return groups;
  }

  async function bulkDelete() {
    await bulkMove("Trash", t("toastDeleted"));
  }

  async function bulkArchive() {
    await bulkMove("Archive", t("toastArchived"));
  }

  async function bulkSpam() {
    await bulkMove(spamFolder, t("toastSpam"));
  }

  function moveSelectedTo(destination: string) {
    return bulkMove(destination, t("toastMoved"));
  }

  // Bulk actions act on what the selected rows represent: with the
  // conversation view on, each checked row is a whole thread, so the batch
  // expands every selected thread into its members (per-folder, so
  // cross-folder selections keep their source attribution). Flat mode moves
  // exactly the checked messages.
  async function bulkMove(destination: string, successLabel: string) {
    const rows = selectedRows();
    if (!conversation) return moveGroups(groupRowsByFolder(rows), destination, successLabel);
    const groups = new Map<string, number[]>();
    const push = (f: string, u: number) => {
      const list = groups.get(f);
      if (list) {
        if (!list.includes(u)) list.push(u);
      } else groups.set(f, [u]);
    };
    for (const m of rows) {
      const src = m.folder || folder;
      if (m.thread_id && m.thread_count && m.thread_count > 1) {
        try {
          const th = await mailThread(src, m.thread_id);
          const members = normalizeThread(th ?? { thread_id: m.thread_id, messages: [] }).messages;
          if (members.length) {
            for (const x of members) push(src, x.uid);
            continue;
          }
        } catch {
          // Listing the thread failed; move the checked row only.
        }
      }
      push(src, m.uid);
    }
    return moveGroups(groups, destination, successLabel);
  }

  function moveDetailTo(destination: string) {
    if (detail) return moveTo([detail.uid], destination, t("toastMoved"));
  }

  async function bulkFlag(flag: string, value: boolean) {
    const rows = selectedRows();
    setError("");
    try {
      const groups = groupRowsByFolder(rows);
      await Promise.all([...groups].map(([src, us]) => us.map((uid) => mailFlag(src, uid, flag, value))).flat());
      refreshUnseen();
      setMessages((ms) =>
        ms.map((m) =>
          selectedUids.has(selectionKey(m, folder))
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
      // Rows flip optimistically but the sidebar badge only knows via the
      // unseen count — refresh it (setSeen and bulkFlag already do this).
      refreshUnseen();
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
        // The muted message just left the folder; if mute was invoked from
        // the reading pane, stop rendering it (mirrors moveTo's cleanup).
        if (selected && selected.uid === m.uid) {
          setSelected(null);
          setDetail(null);
          router.push(`/mail/${encodeURIComponent(folder)}`);
        }
        showToast(t(muted ? "toastUnmuted" : "toastMuted"));
      } catch (e) {
        setError(e instanceof Error ? e.message : "mute failed");
      }
      return;
    }
    try {
      const th = await mailThread(m.folder || folder, m.thread_id);
      const msgs = normalizeThread(th ?? { thread_id: m.thread_id, messages: [] }).messages;
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
      // Same reading-pane cleanup as the single-message branch: the thread
      // moved to Archive, so the open pane must not keep rendering it.
      if (selected && (uids.includes(selected.uid) || selected.thread_id === m.thread_id)) {
        setSelected(null);
        setDetail(null);
        router.push(`/mail/${encodeURIComponent(folder)}`);
      }
      showToast(t(muted ? "toastUnmuted" : "toastMuted"));
    } catch (e) {
      setError(e instanceof Error ? e.message : "mute failed");
    }
  }

  return {
    moveTo, moveSelectedTo, moveDetailTo,
    archiveMessage, spamMessage, removeMessage, removeSingleMessage,
    bulkDelete, bulkArchive, bulkSpam, bulkFlag,
    setSeen, toggleRead, toggleStar, togglePin, toggleMute,
    openMessage, reportNotSpam, reportNotSpamBulk, unsubscribeAction,
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
