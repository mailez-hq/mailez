"use client";

import { useTranslations } from "next-intl";
import { PenLine, Sparkles, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { HAS_OPTIONAL_MODULES } from "@/lib/api";
import { cn } from "@/lib/utils";

// SidebarHeader sits under the global AppHeader (which now owns the brand and
// the account/sign-out menu). It only holds the compose actions and, on small
// screens, a drawer-close control for the mobile drawer.
export function SidebarHeader({
  onClose,
  onCompose,
  aiComposeEnabled,
  aiComposeLocked,
  aiComposeBusy,
  onAiCompose,
}: {
  onClose: () => void;
  onCompose: () => void;
  aiComposeEnabled: boolean;
  aiComposeLocked?: boolean;
  aiComposeBusy: boolean;
  onAiCompose: () => void;
}) {
  const t = useTranslations("mail");

  return (
    <>
      {/* Mobile drawer close: while the drawer is open it covers the app
          header, so it carries its own way out (desktop lg+ is static and
          never needs this). */}
      <div className="flex items-center justify-end px-2 pt-1 lg:hidden">
        <Button variant="ghost" size="icon-sm" onClick={onClose} title={t("close")}>
          <X className="size-4" />
          <span className="sr-only">{t("close")}</span>
        </Button>
      </div>

      {/* Compose actions: 写邮件 + AI 写邮件 at the very top of the sidebar */}
      <div className="flex gap-1.5 px-3 pb-2">
        <Button className="min-w-0 flex-1" onClick={onCompose}>
          <PenLine className="size-4" />
          {t("write")}
        </Button>
        {aiComposeEnabled ? (
          <Button
            variant="outline"
            size="icon"
            className="shrink-0"
            onClick={onAiCompose}
            disabled={aiComposeBusy}
            title={t("aiCompose")}
          >
            <Sparkles className={cn("size-3.5 shrink-0 text-ai", aiComposeBusy && "animate-pulse")} />
            <span className="sr-only">{t("aiCompose")}</span>
          </Button>
        ) : aiComposeLocked && HAS_OPTIONAL_MODULES ? (
          /* Paid-edition build without a configured provider: keep the locked
             entry discoverable. In the community build the AI module does not
             exist, so render nothing instead of a dead locked button. */
          <Button
            variant="outline"
            size="icon"
            className="shrink-0 opacity-60"
            disabled
            title={t("aiLockedTitle")}
          >
            <Sparkles className="size-3.5 shrink-0" />
            <span className="sr-only">{t("aiLockedTitle")}</span>
          </Button>
        ) : null}
      </div>
    </>
  );
}
