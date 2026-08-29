"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";

import {
  aiComposeDraft, aiComposeStream, aiDraft, aiDraftNew,
  contacts, mailDelete, mailIdentities, mailSaveDraft, mailSend, mailUndoSend, mailMerge,
  pgpEncrypt, pgpLookup, pgpSign,
  uploadLargeAttachment,
  type Contact, type DraftTone, type MailIdentity, type MailMessage, type Me, type OutboundAttachment,
} from "@/lib/api";
import {
  MAX_ATTACHMENT_BYTES,
  escHtml,
  normalizeQuoteBody,
  readFileAsBase64,
  stripCollapseMarkers,
  textToHtml,
} from "@/components/mailbox/mail-utils";
import { composeSignature } from "@/lib/compose-signature";
import { parseMergeRecipients } from "@/lib/mail-merge";
import type { MailToastLink } from "./use-toast";

function fmtDate(d: string) {
  return new Date(d).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

/**
 * Compose cluster: the compose panel form, draft persistence (auto-save,
 * manual save, close-save), reply/forward seeding, attachments, identities
 * and signatures, AI draft/compose, and the send pipeline (PGP, mail merge,
 * scheduled send, undo window).
 *
 * `t` comes from the same provider the store uses; `showToast` is threaded
 * in from the store so compose confirmations land on the one toast instance
 * MailShell renders.
 */
export function useCompose({
  folder,
  detail,
  me,
  prefs,
  loadMessages,
  refreshUnseen,
  showToast,
}: {
  folder: string;
  detail: MailMessage | null;
  me: Me;
  prefs: {
    autoSignature: boolean;
    collapseReplyQuote: boolean;
    undoSendSeconds: number;
  };
  loadMessages: (folder: string, page?: number, silent?: boolean) => Promise<void>;
  refreshUnseen: () => void;
  // Shared from the mail store: the toast state that MailShell actually
  // renders. useToast() here would create a second, unrendered instance.
  showToast: (label: string, onUndo?: () => void, duration?: number, link?: MailToastLink) => void;
}) {
  const t = useTranslations("mail");

  // composeError / composeNotice are scoped to the compose panel only, so
  // AI draft/compose and send failures never leak into the message list or
  // the reading pane.
  const [composeError, setComposeError] = useState("");
  const [composeNotice, setComposeNotice] = useState("");

  // ---- compose: form fields ----
  const [composeOpen, setComposeOpen] = useState(false);
  const [receiptOn, setReceiptOn] = useState(false);
  const [mergeOn, setMergeOn] = useState(false);
  const [mergeText, setMergeText] = useState("");
  const [burnAfter, setBurnAfter] = useState(0);
  // Where the initial focus should land when the compose dialog opens:
  // "to" (new message / forward) or "editor" (reply / reply all).
  const [composeFocus, setComposeFocus] = useState<"to" | "editor">("to");
  // hasReplyTarget marks a reply/forward compose (original email available as
  // AI context). New-mail compose leaves it false and AI drafts from subject
  // + hints instead.
  const [hasReplyTarget, setHasReplyTarget] = useState(false);
  const [aiDraftHint, setAiDraftHint] = useState("");
  const [aiComposeBusy, setAiComposeBusy] = useState(false);
  const [drafting, setDrafting] = useState(false);
  const [to, setTo] = useState<string[]>([]);
  const [cc, setCc] = useState<string[]>([]);
  const [bcc, setBcc] = useState<string[]>([]);
  const [ccExpanded, setCcExpanded] = useState(false);
  const [attachments, setAttachments] = useState<OutboundAttachment[]>([]);
  const [dragOverCompose, setDragOverCompose] = useState(false);
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [bodyText, setBodyText] = useState("");
  const [draftTone, setDraftTone] = useState<DraftTone>("formal");
  const [signOn, setSignOn] = useState(false);
  const [encryptOn, setEncryptOn] = useState(false);
  const [draftSaved, setDraftSaved] = useState(false);
  // Optional RFC3339 timestamp (from the compose datetime input) that turns a
  // send into a scheduled send; null means "send now".
  const [scheduleAt, setScheduleAt] = useState<string>("");
  const [allContacts, setAllContacts] = useState<Contact[] | null>(null);
  const [identities, setIdentities] = useState<MailIdentity[]>([]);
  const [from, setFrom] = useState("");

  const toInputRef = useRef<HTMLInputElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  // Snapshot of a closed-but-unsaved-completely compose session: closing the
  // editor persists the draft and remembers its content so the next "Write"
  // restores it instead of starting blank.
  const lastDraftRef = useRef<{
    to: string[]; cc: string[]; bcc: string[]; subject: string; body: string; bodyText: string;
    attachments: OutboundAttachment[]; uid: number | null;
  } | null>(null);
  const draftUidRef = useRef<number | null>(null);
  // Threading headers of the message this compose session replies to, so
  // full-compose replies stay in the conversation exactly like quick replies.
  const replyHeadersRef = useRef<{ inReplyTo: string; references: string } | null>(null);
  const draftBaselineRef = useRef("");
  const draftTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Serializes draft saves: while a save is in flight the draft UID is not yet
  // known, so a second overlapping save would create a duplicate draft instead
  // of updating the first one.
  const draftSavingRef = useRef(false);

  // load the From identities (own address + aliases with DKIM status)
  useEffect(() => {
    mailIdentities()
      .then((ids) => {
        setIdentities(ids);
        setFrom((f) => f || ids[0]?.email || me.email);
      })
      .catch(() => setFrom(me.email));
  }, [me.email]);

  // quoteText builds the quoted original message used in replies/forwards.
  const quoteText = (d: MailMessage) => {
    const from = d.from.map((a) => a.name || a.email).join(", ");
    const lines = normalizeQuoteBody(d.text_body || "").trim();
    if (!lines) return "";
    const quoted = lines.split("\n").map((l) => `> ${l}`).join("\n");
    return `\n\nOn ${fmtDate(d.date)}, ${from} wrote:\n${quoted}`;
  };

  // selectedQuote quotes only the user's current selection in the reading
  // pane when replying; without a selection it falls back to the full body.
  const selectedQuote = (d: MailMessage) => {
    const sel = typeof window !== "undefined" ? window.getSelection()?.toString().trim() : "";
    if (sel && sel.length > 1 && !/^[\r\n\s]+$/.test(sel)) {
      const from = d.from.map((a) => a.name || a.email).join(", ");
      const quoted = normalizeQuoteBody(sel).split("\n").map((l) => `> ${l}`).join("\n");
      return `\n\nOn ${fmtDate(d.date)}, ${from} wrote:\n${quoted}`;
    }
    return quoteText(d);
  };

  // Focus the recipient field when the compose dialog opens in "to" mode
  // (new message / forward). A short delay keeps the focus from being
  // stolen by Base UI's dialog focus management.
  useEffect(() => {
    if (!composeOpen || composeFocus !== "to") return;
    const tm = setTimeout(() => toInputRef.current?.focus(), 60);
    return () => clearTimeout(tm);
  }, [composeOpen, composeFocus]);

  // A draft save lands in the Drafts folder; if the user is browsing Drafts,
  // silently reload its list so the new/updated draft appears right away.
  const refreshDraftsIfActive = () => {
    if (folder === "Drafts") void loadMessages("Drafts", 0, true);
  };

  // Auto-save the draft 30s after the user stops typing, replacing the
  // previous auto-save so one compose session keeps exactly one draft.
  // A pristine reply/forward (only the auto-generated quote, nothing typed)
  // is not user content and must not create a draft on its own.
  useEffect(() => {
    if (!composeOpen) return;
    const hasContent =
      to.length > 0 || cc.length > 0 || subject.trim() !== "" || bodyText.trim() !== "" || attachments.length > 0;
    if (!hasContent) return;
    const pristine =
      draftUidRef.current == null &&
      draftBaselineRef.current ===
        composeSignature({to, cc, bcc, subject, body, bodyText, attachments});
    if (pristine) return;
    setDraftSaved(false);
    if (draftTimerRef.current) clearTimeout(draftTimerRef.current);
    draftTimerRef.current = setTimeout(async () => {
      if (draftSavingRef.current) return; // a manual save already persists this content
      draftSavingRef.current = true;
      try {
        const res = await mailSaveDraft(subject, bodyText, body, draftUidRef.current ?? 0, to, cc, bcc, attachments);
        draftUidRef.current = res.uid || draftUidRef.current;
        setDraftSaved(true);
      } catch {
        // silent: keep editing, the next idle window retries
      } finally {
        draftSavingRef.current = false;
      }
    }, 30000);
    return () => {
      if (draftTimerRef.current) clearTimeout(draftTimerRef.current);
    };
  }, [composeOpen, to, cc, bcc, subject, bodyText, body, attachments]);

  // saveDraftNow writes the draft immediately (manual "save draft" button);
  // auto-save also runs 30s after the user stops typing.
  async function saveDraftNow() {
    const hasContent =
      to.length > 0 || cc.length > 0 || bcc.length > 0 || subject.trim() !== "" || bodyText.trim() !== "" || attachments.length > 0;
    if (!hasContent) return;
    if (draftSavingRef.current) return; // ignore rapid repeated clicks; the first save persists
    draftSavingRef.current = true;
    setComposeError("");
    try {
      const res = await mailSaveDraft(subject, bodyText, body, draftUidRef.current ?? 0, to, cc, bcc, attachments);
      draftUidRef.current = res.uid || draftUidRef.current;
      setDraftSaved(true);
      refreshDraftsIfActive();
    } catch (e) {
      setComposeError(e instanceof Error ? e.message : "save draft failed");
    } finally {
      draftSavingRef.current = false;
    }
  }

  // closeCompose saves the draft (fire-and-forget) and dismisses the panel.
  // Without this, closing within the 30s auto-save window would lose edits.
  function closeCompose() {
    const hasContent =
      to.length > 0 || cc.length > 0 || bcc.length > 0 || subject.trim() !== "" || bodyText.trim() !== "" || attachments.length > 0;
    // Closing a pristine reply (auto quote, no edits, no existing draft)
    // dismisses it without polluting the Drafts folder.
    const pristine =
      draftUidRef.current == null &&
      draftBaselineRef.current ===
        composeSignature({to, cc, bcc, subject, body, bodyText, attachments});
    // A create (uid == null) must not race another in-flight create, otherwise
    // two drafts appear; updating an existing draft is always safe.
    if (hasContent && !pristine && (draftUidRef.current != null || !draftSavingRef.current)) {
      // Remember the content synchronously so a quick reopen restores it even
      // before the async save resolves.
      lastDraftRef.current = {
        to, cc, bcc, subject, body, bodyText, attachments,
        uid: draftUidRef.current,
      };
      mailSaveDraft(subject, bodyText, body, draftUidRef.current ?? 0, to, cc, bcc, attachments)
        .then((res) => {
          draftUidRef.current = res.uid || draftUidRef.current;
          if (lastDraftRef.current) lastDraftRef.current.uid = draftUidRef.current;
          refreshDraftsIfActive();
        })
        .catch(() => {});
    } else {
      lastDraftRef.current = null;
    }
    setComposeOpen(false);
  }

  function openCompose(
    toAddr = "",
    subj = "",
    html = "",
    text = "",
    focus: "to" | "editor" = "to",
    replyTarget = false,
    replyHeaders?: { inReplyTo: string; references: string },
  ) {
    setHasReplyTarget(replyTarget);
    replyHeadersRef.current = replyHeaders ?? null;
    setComposeError("");
    setComposeNotice("");
    // A bare "Write" right after closing a draft restores the saved content
    // instead of starting from scratch (the close path persisted it).
    if (!toAddr && !subj && !html && !text && lastDraftRef.current) {
      const s = lastDraftRef.current;
      lastDraftRef.current = null;
      setTo(s.to);
      setCc(s.cc);
      setBcc(s.bcc);
      setCcExpanded(false);
      setAttachments(s.attachments);
      setSubject(s.subject);
      setBody(s.body);
      setBodyText(s.bodyText);
      setSignOn(false);
      setEncryptOn(false);
      setDraftSaved(false);
      draftUidRef.current = s.uid ?? draftUidRef.current;
      draftBaselineRef.current = composeSignature({
        to: s.to, cc: s.cc, bcc: s.bcc, subject: s.subject, body: s.body, bodyText: s.bodyText, attachments: s.attachments,
      });
      setComposeFocus(focus);
      setComposeOpen(true);
      return;
    }
    const identity = identities.find((i) => i.email === from);
    const sig = prefs.autoSignature
      ? identity?.signature?.trim() || me.signature?.trim()
      : "";
    let finalHtml = html;
    let finalText = text;
    if (sig) {
      const sigHtml = sig
        .split("\n")
        .map((l) => (l.trim() ? `<p>${escHtml(l)}</p>` : "<p><br></p>"))
        .join("");
      finalHtml = `${html}<p><br></p><p>--</p>${sigHtml}`;
      finalText = text ? `${text}\n\n-- \n${sig}` : `-- \n${sig}`;
    }
    const parsedTo = toAddr ? toAddr.split(",").map((s) => s.trim()).filter(Boolean) : [];
    setTo(parsedTo);
    setCc([]);
    setBcc([]);
    setCcExpanded(false);
    setAttachments([]);
    setSubject(subj);
    setBody(finalHtml);
    setBodyText(finalText);
    setSignOn(false);
    setEncryptOn(false);
    setDraftSaved(false);
    draftUidRef.current = null;
    // Baseline for pristine-reply detection: only real user edits (or an
    // existing draft) should trigger auto/close-save.
    draftBaselineRef.current = composeSignature({
      to: parsedTo,
      cc: [],
      bcc: [],
      subject: subj,
      body: finalHtml,
      bodyText: finalText,
      attachments: [],
    });
    setComposeFocus(focus);
    setComposeOpen(true);
  }

  async function addFiles(list: FileList | File[]) {
    try {
      const files = Array.from(list);
      const inline: File[] = [];
      for (const f of files) {
        if (f.size > MAX_ATTACHMENT_BYTES) {
          // 超大附件：relay the file to the server and embed a download link.
          try {
            const up = await uploadLargeAttachment(f);
            const link = `${up.filename}（超大附件）\n下载：${up.url}`;
            setBodyText((prev) => `${prev}\n\n${link}`);
            setBody((prev) => `${prev}<p>${escHtml(up.filename)}（超大附件）<br/><a href="${escHtml(up.url)}">下载</a></p>`);
            showToast(t("largeAttachmentUploaded", { name: up.filename }));
          } catch {
            setComposeError(t("largeAttachmentFailed", { name: f.name }));
          }
          continue;
        }
        inline.push(f);
      }
      if (inline.length === 0) return;
      const ready = await Promise.all(inline.map(readFileAsBase64));
      setAttachments((prev) => [...prev, ...ready]);
      setComposeError("");
    } catch {
      setComposeError(t("attachFailed"));
    }
  }

  function replyAllFrom(m: MailMessage) {
    const recipients = new Set<string>();
    [...m.from, ...(m.cc || []), ...(m.to || [])].forEach((a) => {
      if (a.email && a.email.toLowerCase() !== me.email.toLowerCase()) recipients.add(a.email);
    });
    openCompose(
      [...recipients].join(", "),
      m.subject.startsWith("Re:") ? m.subject : `Re: ${m.subject}`,
      textToHtml(quoteText(m), {collapseQuote: prefs.collapseReplyQuote}),
      quoteText(m),
      "editor",
      true,
      { inReplyTo: m.id, references: m.id },
    );
  }
  // replyFrom / forwardFrom open compose for an arbitrary message (thread
  // members, row context menus) without first swapping the open detail, so
  // the thread reading-pane action buttons work on the clicked message.
  function replyFrom(m: MailMessage) {
    const quote = quoteText(m);
    openCompose(
      m.from[0]?.email || "",
      m.subject.startsWith("Re:") ? m.subject : `Re: ${m.subject}`,
      textToHtml(quote, {collapseQuote: prefs.collapseReplyQuote}),
      quote,
      "editor",
      true,
      { inReplyTo: m.id, references: m.id },
    );
  }

  function forwardFrom(m: MailMessage) {
    const from = m.from.map((a) => a.name || a.email).join(", ");
    const head = `---------- Forwarded message ----------\nFrom: ${from}\nDate: ${fmtDate(m.date)}\nSubject: ${m.subject}\n\n`;
    const quote = head + normalizeQuoteBody(m.text_body || "");
    openCompose(
      "",
      m.subject.startsWith("Fwd:") ? m.subject : `Fwd: ${m.subject}`,
      textToHtml(quote),
      quote,
      "to",
      true,
    );
  }

  // Swapping the From identity replaces the appended signature in place.
  function applySignature(text: string, html: string, sig: string) {
    const sigHtml = sig
      .split("\n")
      .map((l) => (l.trim() ? `<p>${escHtml(l)}</p>` : "<p><br></p>"))
      .join("");
    const cleanText = text.replace(/\n\n-- \n[\s\S]*$/, "");
    const marker = "<p><br></p><p>--</p>";
    const cleanHtml = (() => {
      const idx = html.lastIndexOf(marker);
      return idx >= 0 ? html.slice(0, idx) : html;
    })();
    return {
      text: cleanText ? `${cleanText}\n\n-- \n${sig}` : `-- \n${sig}`,
      html: `${cleanHtml}${marker}${sigHtml}`,
    };
  }

  function selectIdentity(email: string) {
    setFrom(email);
    const idn = identities.find((i) => i.email === email);
    const sig = prefs.autoSignature ? idn?.signature?.trim() : "";
    if (sig) {
      const next = applySignature(bodyText, body, sig);
      setBodyText(next.text);
      setBody(next.html);
    }
  }

  // Recipient auto-suggest: lazily load the address book once and match the
  // last comma-separated token typed in the To field.
  function loadContactsOnce() {
    if (allContacts === null) {
      contacts().then(setAllContacts).catch(() => {});
    }
  }

  function reply() {
    if (!detail) return;
    const quote = selectedQuote(detail);
    openCompose(
      detail.from[0]?.email || "",
      detail.subject.startsWith("Re:") ? detail.subject : `Re: ${detail.subject}`,
      textToHtml(quote, {collapseQuote: prefs.collapseReplyQuote}),
      quote,
      "editor",
      true,
      // Keep the reply in the thread: the backend decodes the routable id
      // back into the raw Message-ID header.
      { inReplyTo: detail.id, references: detail.id },
    );
  }

  // editDraft reopens a saved draft in the compose editor and takes over its
  // uid, so the next save replaces the same draft instead of creating a new
  // one. Attachments are restored when the detail carries their payload.
  function editDraft() {
    if (!detail) return;
    const m = detail;
    const to = (m.to || []).map((a) => a.email).filter(Boolean);
    const cc = (m.cc || []).map((a) => a.email).filter(Boolean);
    // Blind recipients live only on the Drafts copy (backend maps the Bcc
    // envelope); restore them so reopening keeps the full compose state.
    const bcc = (m.bcc || []).map((a) => a.email).filter(Boolean);
    const html = m.html_body || textToHtml(m.text_body || "");
    const text = m.text_body || "";
    const atts = (m.attachments || [])
      .filter((a) => a.data)
      .map((a) => ({ filename: a.filename, content_type: a.content_type, size: a.size, data: a.data as string }));
    setTo(to);
    setCc(cc);
    setBcc(bcc);
    setCcExpanded(cc.length > 0 || bcc.length > 0);
    setAttachments(atts);
    setSubject(m.subject || "");
    setBody(html);
    setBodyText(text);
    setSignOn(false);
    setEncryptOn(false);
    setDraftSaved(false);
    draftUidRef.current = m.uid;
    replyHeadersRef.current = null;
    draftBaselineRef.current = composeSignature({
      to, cc, bcc, subject: m.subject || "", body: html, bodyText: text, attachments: atts,
    });
    setComposeFocus("editor");
    setComposeOpen(true);
  }

  // replyWithQuote opens a reply whose body quotes only the selected passage.
  function replyWithQuote(selection: string) {
    if (!detail) return;
    const s = selection.trim();
    const quote = s ? `> ${s.replace(/\n/g, "\n> ")}\n\n` : selectedQuote(detail);
    openCompose(
      detail.from[0]?.email || "",
      detail.subject.startsWith("Re:") ? detail.subject : `Re: ${detail.subject}`,
      textToHtml(quote),
      quote,
      "editor",
      true,
      { inReplyTo: detail.id, references: detail.id },
    );
  }

  function replyAll() {
    if (!detail) return;
    const recipients = new Set<string>();
    // Reply All addresses every participant of the original message except
    // the replier: sender, explicit recipients AND cc. (The row context-menu
    // variant already included cc; this one was dropping it.)
    [...detail.from, ...(detail.cc || []), ...detail.to].forEach((a) => {
      if (a.email && a.email.toLowerCase() !== me.email.toLowerCase()) recipients.add(a.email);
    });
    const quote = selectedQuote(detail);
    openCompose(
      [...recipients].join(", "),
      detail.subject.startsWith("Re:") ? detail.subject : `Re: ${detail.subject}`,
      textToHtml(quote, {collapseQuote: prefs.collapseReplyQuote}),
      quote,
      "editor",
      true,
      { inReplyTo: detail.id, references: detail.id },
    );
  }

  function forward() {
    if (!detail) return;
    const from = detail.from.map((a) => a.name || a.email).join(", ");
    const head = `---------- Forwarded message ----------\nFrom: ${from}\nDate: ${fmtDate(detail.date)}\nSubject: ${detail.subject}\n\n`;
    const quote = head + normalizeQuoteBody(detail.text_body || "");
    openCompose(
      "",
      detail.subject.startsWith("Fwd:") ? detail.subject : `Fwd: ${detail.subject}`,
      textToHtml(quote),
      quote,
      "to",
      true,
    );
  }

  async function aiDraftReply() {
    setDrafting(true);
    setComposeError("");
    try {
      let res: { draft: string };
      if (hasReplyTarget && detail) {
        // Reply/forward: the original email body is the AI context.
        res = await aiDraft(detail.text_body || detail.html_body || "", draftTone);
      } else {
        // New mail: draft from the subject + what the sender wants to say.
        if (!subject.trim() && !aiDraftHint.trim()) {
          setComposeError(t("aiDraftNeedsInput"));
          setDrafting(false);
          return;
        }
        res = await aiDraftNew(subject, aiDraftHint, draftTone);
      }
      setBody(textToHtml(res.draft));
      setBodyText(res.draft);
    } catch (e) {
      setComposeError(e instanceof Error ? e.message : "ai draft failed");
    } finally {
      setDrafting(false);
    }
  }

  // aiCompose turns a free-form instruction ("给小明写封邮件，说我下周不去
  // 旅游了") into a complete compose. The panel opens immediately and the
  // recipients/subject/body stream in while the model is still writing.
  async function aiCompose(instruction: string) {
    setComposeError("");
    setComposeNotice("");
    setAiComposeBusy(true);
    // Fresh compose: clear the form, then open the panel so the user watches
    // the email being written.
    setTo([]);
    setCc([]);
    setBcc([]);
    setSubject("");
    setBody("");
    setBodyText("");
    draftBaselineRef.current = composeSignature({
      to: [], cc: [], bcc: [], subject: "", body: "", bodyText: "", attachments,
    });
    setComposeOpen(true);
    setComposeFocus("editor");
    try {
      let contactsList = allContacts;
      if (!contactsList) {
        try {
          contactsList = await contacts();
        } catch {
          contactsList = [];
        }
      }
      const bodyParts: string[] = [];
      let subjectAcc = "";
      const flushBody = () => {
        const text = bodyParts.join("");
        setBodyText(text);
        setBody(textToHtml(text));
      };
      // Throttle editor updates so long outputs stay smooth.
      const timer = setInterval(flushBody, 80);
      let gotContent = false;
      await aiComposeStream(instruction, (ev) => {
        if (ev.type === "to" || ev.type === "subject" || ev.type === "body") {
          gotContent = true;
        }
        if (ev.type === "to" && ev.text) {
          const resolved: string[] = [];
          const unresolved: string[] = [];
          for (const raw of ev.text.split(",")) {
            const name = (raw || "").trim();
            if (!name) continue;
            if (name.includes("@")) {
              resolved.push(name);
              continue;
            }
            const hit = (contactsList || []).find(
              (c) =>
                (c.name || "").toLowerCase() === name.toLowerCase() ||
                (c.name || "").toLowerCase().includes(name.toLowerCase()),
            );
            if (hit) resolved.push(hit.email);
            else unresolved.push(name);
          }
          if (resolved.length > 0) setTo(resolved);
          if (unresolved.length > 0) {
            setComposeNotice(t("aiComposeUnresolved", { names: unresolved.join("、") }));
          }
        } else if (ev.type === "subject" && ev.text) {
          subjectAcc += ev.text;
          setSubject(subjectAcc);
        } else if (ev.type === "body" && ev.text) {
          bodyParts.push(ev.text);
        } else if (ev.type === "error" && ev.text) {
          setComposeError(ev.text);
        }
      });
      clearInterval(timer);
      flushBody();
      if (!gotContent) {
        // Streaming produced nothing (proxy buffering or an empty reply):
        // fall back to the one-shot endpoint so the user still gets the mail.
        const draft = await aiComposeDraft(instruction);
        const resolved: string[] = [];
        const unresolved: string[] = [];
        for (const raw of draft.to || []) {
          const name = (raw || "").trim();
          if (!name) continue;
          if (name.includes("@")) {
            resolved.push(name);
            continue;
          }
          const hit = (contactsList || []).find(
            (c) =>
              (c.name || "").toLowerCase() === name.toLowerCase() ||
              (c.name || "").toLowerCase().includes(name.toLowerCase()),
          );
          if (hit) resolved.push(hit.email);
          else unresolved.push(name);
        }
        if (resolved.length > 0) setTo(resolved);
        if (unresolved.length > 0) {
          setComposeNotice(t("aiComposeUnresolved", { names: unresolved.join("、") }));
        }
        if (draft.subject) setSubject(draft.subject);
        if (draft.body) {
          setBodyText(draft.body);
          setBody(textToHtml(draft.body));
        }
      }
    } catch (e) {
      setComposeError(e instanceof Error ? e.message : t("aiComposeFailed"));
    } finally {
      setAiComposeBusy(false);
    }
  }

  async function send(e: React.FormEvent) {
    e.preventDefault();
    setComposeError("");
    try {
      let text = bodyText;
      let html = stripCollapseMarkers(body);
      if (signOn) {
        const { signature } = await pgpSign(text || " ");
        text = `${text}\n\n${signature}`;
        html = textToHtml(text);
      }
      if (encryptOn) {
        const recipient = to[0]?.trim();
        if (!recipient) throw new Error(t("pgpNoRecipient"));
        let publicKey: string;
        try {
          publicKey = (await pgpLookup(recipient)).public_key;
        } catch {
          throw new Error(t("pgpNoKeyForRecipient"));
        }
        const { encrypted } = await pgpEncrypt(text || " ", publicKey);
        text = encrypted;
        html = "";
      }

      const finalTo = [...to];
      const finalCc = [...cc];
      const finalBcc = [...bcc];
      const finalAttachments = [...attachments];
      const finalSubject = subject;
      const finalText = text;
      const finalHtml = html;
      const finalFrom = from;
      const delay = prefs.undoSendSeconds;

      // Mail merge (逐封群发): each line is "email, 姓名" and the subject/body
      // may reference {{name}}, {{email}} and custom {{var}} placeholders.
      if (mergeOn) {
        const recipients = parseMergeRecipients(mergeText);
        if (recipients.length === 0) {
          setComposeError(t("mergeNoRecipients"));
          return;
        }
        const res = await mailMerge({
          subject: finalSubject,
          body: finalText,
          html: finalHtml,
          from: finalFrom,
          recipients,
        });
        setMergeText("");
        setMergeOn(false);
        closeCompose();
        showToast(t("mergeSent", { count: res.sent }));
        if (res.failed.length > 0) {
          setComposeError(t("mergeFailed", { count: res.failed.length }));
        }
        return;
      }
      // datetime-local value -> RFC3339 (interpreted as local time).
      const sendAt = scheduleAt ? new Date(scheduleAt).toISOString() : undefined;

      const resetCompose = () => {
        lastDraftRef.current = null;
        setComposeOpen(false);
        setTo([]);
        setCc([]);
        setBcc([]);
        setCcExpanded(false);
        setAttachments([]);
        setSubject("");
        setBody("");
        setBodyText("");
        setScheduleAt("");
        setDraftSaved(false);
        draftUidRef.current = null;
        replyHeadersRef.current = null;
      };

      // Sending an edited draft must remove the original from Drafts; the
      // backend only handles the outbox, not draft cleanup.
      const removeEditedDraft = async (draftUid: number | null) => {
        if (!draftUid) return;
        try {
          await mailDelete("Drafts", draftUid);
          refreshDraftsIfActive();
        } catch {
          // The draft may already be gone; the next folder load reconciles.
        }
      };

      // Scheduled send: the backend parks the message until send_at. Cancel it
      // from the Scheduled dialog, not the send/undo toast.
      if (sendAt) {
        await mailSend(finalTo, finalCc, finalBcc, finalSubject, finalText, finalHtml, finalFrom, finalAttachments, 0, sendAt, receiptOn, burnAfter, replyHeadersRef.current?.inReplyTo, replyHeadersRef.current?.references);
        removeEditedDraft(draftUidRef.current);
        resetCompose();
        showToast(t("toastScheduled", { time: fmtDate(sendAt) }));
        return;
      }

      if (delay <= 0) {
        await mailSend(finalTo, finalCc, finalBcc, finalSubject, finalText, finalHtml, finalFrom, finalAttachments, 0, undefined, receiptOn, burnAfter, replyHeadersRef.current?.inReplyTo, replyHeadersRef.current?.references);
        removeEditedDraft(draftUidRef.current);
        resetCompose();
        loadMessages(folder);
        // Immediate send: confirm the outcome. Gmail/Fastmail/Coremail all
        // keep the user where they are and confirm with a toast instead of
        // navigating away — a silent close leaves "did it send?" unanswered.
        showToast(t("toastSent"), undefined, 6000, { label: t("viewSent"), folder: "Sent" });
        return;
      }

      // Server-side undo window: the backend parks the message in its outbox
      // and delivers it when the window elapses, so closing the tab no longer
      // loses the send.
      const res = await mailSend(finalTo, finalCc, finalBcc, finalSubject, finalText, finalHtml, finalFrom, finalAttachments, delay, undefined, receiptOn, burnAfter, replyHeadersRef.current?.inReplyTo, replyHeadersRef.current?.references);
      removeEditedDraft(draftUidRef.current);
      const outboxId = res?.outbox_id;
      resetCompose();

      showToast(t("sending"), () => {
        if (outboxId) mailUndoSend(outboxId).catch(() => {});
      }, delay * 1000);

      // The backend parks the message in the outbox until the undo window
      // elapses, then delivers it. Refresh the current folder after that
      // window so a sent-and-delivered message shows up without requiring a
      // manual refresh, and flip the toast to the final confirmation — the
      // two-phase "Sending… → Sent" rhythm Gmail uses.
      setTimeout(() => {
        loadMessages(folder, 0, true);
        refreshUnseen();
        showToast(t("toastSent"), undefined, 6000, { label: t("viewSent"), folder: "Sent" });
      }, delay * 1000 + 2000);
    } catch (err) {
      setComposeError(err instanceof Error ? err.message : "send failed");
    }
  }

  // Esc dismisses the compose panel (no Base UI dialog to handle it anymore).
  useEffect(() => {
    if (!composeOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") closeCompose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- closeCompose closes over exactly the form state above; its unstable identity would resubscribe every render
  }, [composeOpen]);

  return {
    composeOpen, setComposeOpen,
    composeError, setComposeError,
    composeNotice, setComposeNotice,
    receiptOn, setReceiptOn,
    mergeOn, setMergeOn,
    mergeText, setMergeText,
    burnAfter, setBurnAfter,
    composeFocus,
    hasReplyTarget,
    aiDraftHint, setAiDraftHint,
    aiComposeBusy,
    drafting,
    to, setTo,
    cc, setCc,
    bcc, setBcc,
    ccExpanded, setCcExpanded,
    attachments, setAttachments,
    dragOverCompose, setDragOverCompose,
    subject, setSubject,
    body, setBody,
    bodyText, setBodyText,
    draftTone, setDraftTone,
    signOn, setSignOn,
    encryptOn, setEncryptOn,
    draftSaved, setDraftSaved,
    scheduleAt, setScheduleAt,
    allContacts,
    identities,
    from,
    selectIdentity,
    toInputRef, fileInputRef,
    quoteText,
    saveDraftNow,
    closeCompose,
    openCompose,
    addFiles,
    replyAllFrom, replyFrom, forwardFrom,
    loadContactsOnce,
    reply, editDraft, replyWithQuote, replyAll, forward,
    aiDraftReply, aiCompose,
    send,
  };
}
