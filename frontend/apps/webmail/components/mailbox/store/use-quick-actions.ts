"use client";

import type { Dispatch, SetStateAction } from "react";

import { mailMove, mailRecall, mailRecallApply, mailReadAll, mailReceipt, mailSendReply } from "@/lib/api";
import type { Translate } from "./use-folder-mgmt";

/**
 * Quick-action cluster: one-shot API calls from the reading pane that all
 * share the same shape — call the API, toast the outcome, refresh the view.
 * (Inline quick reply, read receipts, message recall/recall-apply, mark-all.)
 */
export function useQuickActions({
  setError,
  showToast,
  refreshMail,
  t,
}: {
  setError: Dispatch<SetStateAction<string>>;
  showToast: (label: string, onUndo?: () => void, duration?: number) => void;
  refreshMail: () => void | Promise<void>;
  t: Translate;
}) {
  // sendQuickReply sends an inline quick reply from the reading pane without
  // opening the compose panel; threading headers keep it in the conversation.
  async function sendQuickReply(
    to: string[],
    cc: string[],
    subject: string,
    text: string,
    inReplyTo: string,
    references: string,
  ): Promise<boolean> {
    setError("");
    try {
      await mailSendReply(to, cc, subject, text, inReplyTo, references);
      showToast(t("toastSent"));
      refreshMail();
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : "send failed");
      return false;
    }
  }

  // sendReceipt answers a read-receipt request (RFC 3798): the backend mails
  // the disposition notification and marks $MDNSent on the message.
  async function sendReceipt(folderName: string, uid: number): Promise<boolean> {
    setError("");
    try {
      await mailReceipt(folderName, uid);
      showToast(t("receiptSent"));
      refreshMail();
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : "receipt failed");
      return false;
    }
  }

  // recallMessage recalls a sent message: the backend mails X-MS-Recall
  // notices to the recipients and flags the sent copy.
  async function recallMessage(folderName: string, uid: number): Promise<boolean> {
    setError("");
    try {
      await mailRecall(folderName, uid);
      showToast(t("recallSent"));
      refreshMail();
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : "recall failed");
      return false;
    }
  }

  // applyRecall deletes the original message a recall notice targets, then
  // removes the notice itself.
  async function applyRecall(messageId: string, noticeFolder: string, noticeUid: number): Promise<boolean> {
    setError("");
    try {
      const res = await mailRecallApply(messageId);
      if (noticeFolder && noticeUid) {
        await mailMove(noticeFolder, [noticeUid], "Trash").catch(() => {});
      }
      showToast(t("recallApplied", { removed: res.removed }));
      refreshMail();
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : "recall apply failed");
      return false;
    }
  }

  // markAllRead marks the current folder's messages as read.
  async function markAllRead(folderName: string): Promise<boolean> {
    setError("");
    try {
      await mailReadAll(folderName);
      showToast(t("markedAllRead"));
      refreshMail();
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : "mark all read failed");
      return false;
    }
  }

  return { sendQuickReply, sendReceipt, recallMessage, applyRecall, markAllRead };
}
