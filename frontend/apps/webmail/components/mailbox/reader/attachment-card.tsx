"use client";

import { useState } from "react";
import {
  Archive, Eye, File, FileArchive, FileAudio, FileText, FileVideo,
  Image as ImageIcon,
} from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import type { MailAttachment } from "@/lib/api";

export function fmtSize(n: number) {
  if (n >= 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  if (n >= 1024) return `${Math.round(n / 1024)} KB`;
  return `${n} B`;
}

function attachmentIcon(a: MailAttachment) {
  const ct = a.content_type;
  const cls = "size-4 shrink-0";
  if (ct.startsWith("image/")) return <ImageIcon className={cls} />;
  if (ct.startsWith("audio/")) return <FileAudio className={cls} />;
  if (ct.startsWith("video/")) return <FileVideo className={cls} />;
  if (ct.includes("zip") || ct.includes("rar") || ct.includes("tar") || ct.includes("7z"))
    return <FileArchive className={cls} />;
  if (ct.includes("pdf") || ct.startsWith("text/")) return <FileText className={cls} />;
  return <File className={cls} />;
}

export function AttachmentCard({ attachment }: { attachment: MailAttachment }) {
  const t = useTranslations("mail");
  const isImage = attachment.content_type.startsWith("image/") && attachment.data;
  const isPdf = attachment.content_type.includes("pdf") && attachment.data;
  const [previewOpen, setPreviewOpen] = useState(false);
  return (
    <div className="w-48 rounded-lg border border-border p-2 transition-colors hover:bg-muted/50">
      {isImage && (
        // User-supplied data-URI preview: next/image cannot optimize these.
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={`data:${attachment.content_type};base64,${attachment.data}`}
          alt={attachment.filename}
          className="mb-2 max-h-24 w-full rounded-md object-cover"
        />
      )}
      <a
        href={`data:${attachment.content_type};base64,${attachment.data}`}
        download={attachment.filename}
        className="flex items-center gap-2"
      >
        {attachmentIcon(attachment)}
        <span className="min-w-0 flex-1">
          <span className="block truncate text-xs font-medium">{attachment.filename}</span>
          <span className="block text-[11px] text-muted-foreground">
            {fmtSize(attachment.size)}
          </span>
        </span>
        <Archive className="size-3.5 shrink-0 text-muted-foreground" />
      </a>
      {isPdf && (
        <Button
          size="xs"
          variant="outline"
          className="mt-2 w-full"
          onClick={() => setPreviewOpen(true)}
        >
          <Eye className="size-3" />
          {t("preview")}
        </Button>
      )}
      <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
        <DialogContent className="sm:max-w-3xl">
          <DialogHeader><DialogTitle>{attachment.filename}</DialogTitle></DialogHeader>
          <iframe
            src={`data:application/pdf;base64,${attachment.data}`}
            className="h-[70vh] w-full rounded-lg border border-border"
            title={attachment.filename}
          />
        </DialogContent>
      </Dialog>
    </div>
  );
}
