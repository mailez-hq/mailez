"use client";

import { Flame, Loader2, Lock, MailCheck, MailWarning } from "lucide-react";
import { useTranslations } from "next-intl";
import type { Dispatch, SetStateAction } from "react";

import { Button } from "@/components/ui/button";
import { rememberRemoteSender } from "@/components/mailbox/reader/remote-images";
import type { MailMessage } from "@/lib/api";

// ReceiptBanner offers to send the RFC 3798 read receipt requested by the
// sender, until the $MDNSent keyword is set.
export function ReceiptBanner({
  detail,
  folder,
  receiptBusy,
  setReceiptBusy,
  onSendReceipt,
}: {
  detail: MailMessage;
  folder: string;
  receiptBusy: boolean;
  setReceiptBusy: Dispatch<SetStateAction<boolean>>;
  onSendReceipt: (folder: string, uid: number) => Promise<boolean>;
}) {
  const t = useTranslations("mail");
  if (!detail.receipt_requested || detail.flags.includes("$MDNSent")) return null;
  return (
    <div className="mb-3 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/40 p-3 text-xs">
      <MailCheck className="size-4 text-muted-foreground" />
      <span className="flex-1">{t("receiptRequested")}</span>
      <Button
        size="sm"
        variant="outline"
        disabled={receiptBusy}
        onClick={async () => {
          setReceiptBusy(true);
          await onSendReceipt(detail.folder || folder, detail.uid);
          setReceiptBusy(false);
        }}
      >
        {receiptBusy ? <Loader2 className="size-3.5 animate-spin" /> : null}
        {t("sendReceipt")}
      </Button>
    </div>
  );
}

// RecallNotice is the Outlook-style X-MS-Recall banner: the sender asked to
// delete the original, and the reader can apply or dismiss the recall.
export function RecallNotice({
  detail,
  folder,
  recallBusy,
  setRecallBusy,
  recallHandled,
  setRecallHandled,
  onApplyRecall,
}: {
  detail: MailMessage;
  folder: string;
  recallBusy: boolean;
  setRecallBusy: Dispatch<SetStateAction<boolean>>;
  recallHandled: boolean;
  setRecallHandled: Dispatch<SetStateAction<boolean>>;
  onApplyRecall: (messageId: string, folder: string, uid: number) => Promise<boolean>;
}) {
  const t = useTranslations("mail");
  if (!detail.recall || recallHandled) return null;
  return (
    <div className="mb-3 flex flex-wrap items-center gap-2 rounded-lg border border-amber-300/60 bg-amber-50 p-3 text-xs dark:border-amber-700/50 dark:bg-amber-950/30">
      <MailWarning className="size-4 text-amber-600" />
      <span className="flex-1">
        {t("recallNotice", { subject: detail.recall.subject })}
      </span>
      <Button
        size="sm"
        variant="outline"
        disabled={recallBusy}
        onClick={async () => {
          setRecallBusy(true);
          const ok = await onApplyRecall(detail.recall!.message_id, detail.folder || folder, detail.uid);
          if (ok) setRecallHandled(true);
          setRecallBusy(false);
        }}
      >
        {recallBusy ? <Loader2 className="size-3.5 animate-spin" /> : null}
        {t("recallDeleteOriginal")}
      </Button>
      <Button size="sm" variant="ghost" onClick={() => setRecallHandled(true)}>
        {t("recallDismiss")}
      </Button>
    </div>
  );
}

// PgpBanner shows the encryption state of a PGP message and offers the
// passphrase-less decrypt action.
export function PgpBanner({
  isPgpEncrypted,
  pgpPlaintext,
  onDecrypt,
  decrypting,
  pgpError,
}: {
  isPgpEncrypted: boolean;
  pgpPlaintext: string | null;
  onDecrypt: () => void;
  decrypting: boolean;
  pgpError: string;
}) {
  const t = useTranslations("mail");
  if (!isPgpEncrypted) return null;
  return (
    <div className="mb-4 flex flex-wrap items-center gap-2 rounded-lg border border-ai/30 bg-ai/10 px-3 py-2 text-xs text-ai">
      <Lock className="size-3.5" />
      {pgpPlaintext !== null ? (
        <span>{t("pgpDecrypted")}</span>
      ) : (
        <>
          <span>{t("pgpEncrypted")}</span>
          <Button size="xs" variant="outline" onClick={onDecrypt} disabled={decrypting}>
            {decrypting ? (
              <Loader2 className="size-3 animate-spin" />
            ) : (
              <Lock className="size-3" />
            )}
            {decrypting ? t("pgpDecrypting") : t("pgpDecrypt")}
          </Button>
        </>
      )}
      {pgpError && <span className="text-destructive">{pgpError}</span>}
    </div>
  );
}

// RemoteImageBanner warns that the message embeds remote images and lets the
// reader load them once or always for this sender domain.
export function RemoteImageBanner({
  remoteImages,
  remoteLoaded,
  setRemoteLoaded,
  senderDomain,
}: {
  remoteImages: boolean;
  remoteLoaded: boolean;
  setRemoteLoaded: Dispatch<SetStateAction<boolean>>;
  senderDomain: string;
}) {
  const t = useTranslations("mail");
  if (!remoteImages || remoteLoaded) return null;
  return (
    <div className="mb-4 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/50 px-3 py-2 text-xs text-muted-foreground">
      <span>{t("remoteImages")}</span>
      <Button size="xs" variant="outline" onClick={() => setRemoteLoaded(true)}>
        {t("loadRemoteImages")}
      </Button>
      <Button
        size="xs"
        variant="outline"
        onClick={() => {
          rememberRemoteSender(senderDomain);
          setRemoteLoaded(true);
        }}
      >
        {t("alwaysLoadRemote")}
      </Button>
    </div>
  );
}

// TranslationPanel shows the AI translation of the body while the reader is
// viewing the translated version.
export function TranslationPanel({
  translatedView,
  translation,
  setTranslatedView,
}: {
  translatedView: boolean;
  translation: string;
  setTranslatedView: Dispatch<SetStateAction<boolean>>;
}) {
  const t = useTranslations("mail");
  if (!translatedView || !translation) return null;
  return (
    <div className="mb-4 rounded-lg border border-border p-3">
      <div className="mb-1.5 flex items-center justify-between">
        <p className="text-xs font-medium text-muted-foreground">{t("translate")}</p>
        <Button size="xs" variant="ghost" onClick={() => setTranslatedView(false)}>
          {t("showOriginal")}
        </Button>
      </div>
      <div className="whitespace-pre-wrap text-sm leading-6">{translation}</div>
    </div>
  );
}

// BurnGate is the burn-after-read gate: the body stays hidden behind this
// notice until the reader reveals it once, flagging the message $BurnRead.
export function BurnGate({
  detail,
  burnRevealed,
  onReveal,
}: {
  detail: MailMessage;
  burnRevealed: boolean;
  onReveal: () => void;
}) {
  const t = useTranslations("mail");
  if (!(detail.burn_after_minutes && detail.burn_after_minutes > 0 &&
    !detail.flags.includes("$BurnRead") && !burnRevealed)) {
    return null;
  }
  return (
    <div className="relative mb-4 rounded-lg border border-orange-300/60 bg-orange-50 p-6 text-center dark:border-orange-700/50 dark:bg-orange-950/30">
      <Flame className="mx-auto mb-2 size-6 text-orange-500" />
      <p className="text-sm font-medium">{t("burnNotice")}</p>
      <p className="mt-1 text-xs text-muted-foreground">
        {t("burnHint", { minutes: detail.burn_after_minutes })}
      </p>
      <Button className="mt-3" size="sm" onClick={onReveal}>
        {t("burnReveal")}
      </Button>
    </div>
  );
}
