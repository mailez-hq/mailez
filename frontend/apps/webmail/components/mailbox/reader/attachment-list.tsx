"use client";

import { Download } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { AttachmentCard } from "@/components/mailbox/reader/attachment-card";
import type { MailMessage } from "@/lib/api";

// AttachmentList renders the attachment section of a message with the
// download-all-zip action in its header.
export function AttachmentList({
  detail,
  onDownloadAll,
}: {
  detail: MailMessage;
  onDownloadAll: () => void;
}) {
  const t = useTranslations("mail");
  if (!detail.attachments || detail.attachments.length === 0) return null;
  return (
    <div className="mt-4 border-t border-border pt-3">
      <div className="mb-2 flex items-center justify-between">
        <p className="text-xs font-medium text-muted-foreground">
          {t("attachments", { count: detail.attachments.length })}
        </p>
        <Button size="xs" variant="outline" onClick={onDownloadAll}>
          <Download className="size-3" />
          {t("downloadAll")}
        </Button>
      </div>
      <div className="flex flex-wrap gap-2">
        {detail.attachments.map((a, i) => (
          <AttachmentCard key={i} attachment={a} />
        ))}
      </div>
    </div>
  );
}
