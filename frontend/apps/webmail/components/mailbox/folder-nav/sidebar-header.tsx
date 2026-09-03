"use client";

import { useTranslations } from "next-intl";
import Link from "next/link";
import { Menu, PenLine, Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Logo } from "@/components/logo";
import { useBrand } from "@/lib/use-brand";
import { cn } from "@/lib/utils";

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
  // White-label: branded deployments show the org name (and logo when
  // configured) instead of the built-in Mailez wordmark.
  const brand = useBrand();

  return (
    <>
      <div className="flex h-12 items-center justify-between px-3">
        {/* Logo navigates straight into the workspace instead of "/" (the
            sign-in route), so clicking it never flashes the login page or
            re-runs the auth check. */}
        <Link
          href="/home"
          onClick={onClose}
          className="flex items-center gap-2 text-lg font-extrabold tracking-tight text-foreground hover:opacity-80"
        >
          {brand.logo_url ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={brand.logo_url}
              alt={brand.title}
              className="size-8 shrink-0 rounded-lg object-contain"
            />
          ) : (
            <Logo />
          )}
          {brand.title || "Mailez Webmail"}
        </Link>
        <Button variant="ghost" size="sm" onClick={onClose} className="lg:hidden">
          <Menu className="size-4" />
          <span className="sr-only">{t("menu")}</span>
        </Button>
      </div>

      {/* Compose actions: 写邮件 + AI 写邮件, at the very top of the sidebar */}
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
        ) : aiComposeLocked ? (
          /* Community edition: the AI entry stays visible but locked so users
             can see what the enterprise edition adds. */
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
