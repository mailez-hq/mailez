"use client";

import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import type { Accent, Density, Landing, ReaderFontSize, ReadingPaneWidth, Theme } from "@/lib/preferences";
import { cn } from "@/lib/utils";
import { Segmented } from "./segmented";

const ACCENT_COLORS: Record<Accent, string> = {
  blue: "#2E6E8E",
  green: "#2F8E6C",
  purple: "#6B4FA0",
  orange: "#C4712A",
  rose: "#B0555B",
};

// AppearanceSection covers the look-and-feel preferences: theme, landing
// page, density, accent color, reader typography, and reading-pane toggles.
export function AppearanceSection({
  t,
  theme, setTheme,
  landing, setLanding,
  density, setDensity,
  accent, setAccent,
  spellcheck, setSpellcheck,
  readerFont, setReaderFont,
  paneWidth, setPaneWidth,
  conversation, setConversation,
  collapseReplyQuote, setCollapseReplyQuote,
}: {
  t: (key: string) => string;
  theme: Theme; setTheme: (v: Theme) => void;
  landing: Landing; setLanding: (v: Landing) => void;
  density: Density; setDensity: (v: Density) => void;
  accent: Accent; setAccent: (v: Accent) => void;
  spellcheck: boolean; setSpellcheck: (v: boolean) => void;
  readerFont: ReaderFontSize; setReaderFont: (v: ReaderFontSize) => void;
  paneWidth: ReadingPaneWidth; setPaneWidth: (v: ReadingPaneWidth) => void;
  conversation: boolean; setConversation: (v: boolean) => void;
  collapseReplyQuote: boolean; setCollapseReplyQuote: (v: boolean) => void;
}) {
  return (
    <div className="space-y-3">
      <p className="text-sm font-medium">{t("appearance")}</p>
      <div className="space-y-1.5">
        <Label>{t("theme")}</Label>
        <Segmented
          value={theme}
          options={[
            { value: "light", label: t("themeLight") },
            { value: "dark", label: t("themeDark") },
            { value: "system", label: t("themeSystem") },
          ]}
          onChange={(v) => setTheme(v as Theme)}
        />
      </div>
      <div className="space-y-1.5">
        <Label>{t("landing")}</Label>
        <Segmented
          value={landing}
          options={[
            { value: "home", label: t("landingHome") },
            { value: "inbox", label: t("landingInbox") },
          ]}
          onChange={(v) => setLanding(v as Landing)}
        />
      </div>
      <div className="space-y-1.5">
        <Label>{t("density")}</Label>
        <Segmented
          value={density}
          options={[
            { value: "compact", label: t("densityCompact") },
            { value: "cozy", label: t("densityCozy") },
            { value: "relaxed", label: t("densityRelaxed") },
          ]}
          onChange={(v) => setDensity(v as Density)}
        />
      </div>
      <div className="space-y-1.5">
        <Label>{t("accent")}</Label>
        <div className="flex gap-1.5">
          {(["blue", "green", "purple", "orange", "rose"] as Accent[]).map((a) => (
            <button
              key={a}
              type="button"
              onClick={() => setAccent(a)}
              title={t(`accent${a.charAt(0).toUpperCase()}${a.slice(1)}`)}
              className={cn(
                "size-6 rounded-full border-2 transition-transform",
                accent === a ? "scale-110 border-foreground" : "border-transparent hover:scale-105",
              )}
              style={{ backgroundColor: ACCENT_COLORS[a] }}
            />
          ))}
        </div>
      </div>
      <div className="space-y-1.5">
        <Label>{t("readerFont")}</Label>
        <Segmented
          value={readerFont}
          options={[
            { value: "sm", label: t("readerFontSm") },
            { value: "md", label: t("readerFontMd") },
            { value: "lg", label: t("readerFontLg") },
            { value: "xl", label: t("readerFontXl") },
          ]}
          onChange={(v) => setReaderFont(v as ReaderFontSize)}
        />
      </div>
      <div className="space-y-1.5">
        <Label>{t("paneWidth")}</Label>
        <Segmented
          value={paneWidth}
          options={[
            { value: "narrow", label: t("paneNarrow") },
            { value: "md", label: t("paneMd") },
            { value: "wide", label: t("paneWide") },
          ]}
          onChange={(v) => setPaneWidth(v as ReadingPaneWidth)}
        />
      </div>
      <div className="flex items-center justify-between">
        <Label>{t("spellcheck")}</Label>
        <Switch checked={spellcheck} onCheckedChange={setSpellcheck} />
      </div>
      <div className="flex items-center justify-between">
        <div>
          <Label>{t("conversationView")}</Label>
          <p className="text-xs text-muted-foreground">{t("conversationViewHint")}</p>
        </div>
        <Switch checked={conversation} onCheckedChange={setConversation} />
      </div>
      <div className="flex items-center justify-between">
        <div>
          <Label>{t("collapseReplyQuote")}</Label>
          <p className="text-xs text-muted-foreground">{t("collapseReplyQuoteHint")}</p>
        </div>
        <Switch checked={collapseReplyQuote} onCheckedChange={setCollapseReplyQuote} />
      </div>
    </div>
  );
}
