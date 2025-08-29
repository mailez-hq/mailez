"use client";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import type { MeSettings } from "@/lib/api";
import type { Preferences } from "@/lib/preferences";

// The small profile sections. They all live inside the shared profile
// <form> in sections.tsx and persist through its single save button, so
// each only needs its own slice of form state.

export function AiSection({
  t,
  prefs,
  setAi,
}: {
  t: (key: string) => string;
  prefs: Preferences;
  setAi: (ai: Preferences["ai"]) => void;
}) {
  return (
    <div className="space-y-2.5">
      <p className="text-sm font-medium">{t("aiFeatures")}</p>
      <div className="flex items-center justify-between">
        <Label>{t("aiEnabled")}</Label>
        <Switch
          checked={prefs.ai.enabled}
          onCheckedChange={(v) => setAi({ ...prefs.ai, enabled: v })}
        />
      </div>
      {prefs.ai.enabled && (
        <>
          <div className="flex items-center justify-between">
            <Label>{t("aiSummary")}</Label>
            <Switch
              checked={prefs.ai.summary}
              onCheckedChange={(v) => setAi({ ...prefs.ai, summary: v })}
            />
          </div>
          <div className="flex items-center justify-between">
            <Label>{t("aiDraft")}</Label>
            <Switch
              checked={prefs.ai.draft}
              onCheckedChange={(v) => setAi({ ...prefs.ai, draft: v })}
            />
          </div>
          <div className="flex items-center justify-between">
            <Label>{t("aiPriority")}</Label>
            <Switch
              checked={prefs.ai.priority}
              onCheckedChange={(v) => setAi({ ...prefs.ai, priority: v })}
            />
          </div>
          <div className="flex items-center justify-between">
            <Label>{t("aiSearch")}</Label>
            <Switch
              checked={prefs.ai.search}
              onCheckedChange={(v) => setAi({ ...prefs.ai, search: v })}
            />
          </div>
        </>
      )}
      <p className="text-xs text-muted-foreground">{t("aiNote")}</p>
    </div>
  );
}

export function NotificationsSection({
  t,
  prefs,
  setNotifications,
}: {
  t: (key: string) => string;
  prefs: Preferences;
  setNotifications: (v: boolean) => void;
}) {
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">{t("notifications")}</p>
      <div className="flex items-center justify-between">
        <Label>{t("notifications")}</Label>
        <Switch
          checked={prefs.notifications}
          onCheckedChange={(v) => {
            setNotifications(v);
            if (v && typeof Notification !== "undefined" && Notification.permission === "default") {
              Notification.requestPermission().catch(() => {});
            }
          }}
        />
      </div>
    </div>
  );
}

export function IdentitySection({
  t,
  profile,
  displayedName, setDisplayedName,
  signature, setSignature,
  autoSignature, setAutoSignature,
}: {
  t: (key: string) => string;
  profile: MeSettings | null;
  displayedName: string; setDisplayedName: (v: string) => void;
  signature: string; setSignature: (v: string) => void;
  autoSignature: boolean; setAutoSignature: (v: boolean) => void;
}) {
  return (
    <div className="space-y-4">
      <p className="text-sm font-medium">{t("identity")}</p>
      <div className="space-y-2">
        <Label>{t("displayedName")}</Label>
        <Input value={displayedName} onChange={(e) => setDisplayedName(e.target.value)} placeholder={profile?.email} />
      </div>
      <div className="space-y-2">
        <Label>{t("signature")}</Label>
        <textarea
          value={signature}
          onChange={(e) => setSignature(e.target.value)}
          rows={4}
          className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm"
        />
        <p className="text-xs text-muted-foreground">{t("signatureHint")}</p>
      </div>
      <div className="flex items-center justify-between">
        <div>
          <Label>{t("autoSignature")}</Label>
          <p className="text-xs text-muted-foreground">{t("autoSignatureHint")}</p>
        </div>
        <Switch checked={autoSignature} onCheckedChange={setAutoSignature} />
      </div>
    </div>
  );
}

export function ForwardingSection({
  t,
  forwardEnabled, setForwardEnabled,
  forwardDestination, setForwardDestination,
  forwardKeep, setForwardKeep,
}: {
  t: (key: string) => string;
  forwardEnabled: boolean; setForwardEnabled: (v: boolean) => void;
  forwardDestination: string; setForwardDestination: (v: string) => void;
  forwardKeep: boolean; setForwardKeep: (v: boolean) => void;
}) {
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">{t("forwarding")}</p>
      <div className="flex items-center justify-between">
        <Label>{t("forwardMail")}</Label>
        <Switch checked={forwardEnabled} onCheckedChange={setForwardEnabled} />
      </div>
      {forwardEnabled && (
        <>
          <div className="space-y-2">
            <Label>{t("forwardTo")}</Label>
            <Input value={forwardDestination} onChange={(e) => setForwardDestination(e.target.value)} placeholder="other@example.com" />
          </div>
          <div className="flex items-center justify-between">
            <Label>{t("forwardKeep")}</Label>
            <Switch checked={forwardKeep} onCheckedChange={setForwardKeep} />
          </div>
        </>
      )}
    </div>
  );
}

export function AutoReplySection({
  t,
  replyEnabled, setReplyEnabled,
  replySubject, setReplySubject,
  replyBody, setReplyBody,
  replyStartdate, setReplyStartdate,
  replyEnddate, setReplyEnddate,
}: {
  t: (key: string) => string;
  replyEnabled: boolean; setReplyEnabled: (v: boolean) => void;
  replySubject: string; setReplySubject: (v: string) => void;
  replyBody: string; setReplyBody: (v: string) => void;
  replyStartdate: string; setReplyStartdate: (v: string) => void;
  replyEnddate: string; setReplyEnddate: (v: string) => void;
}) {
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">{t("autoReply")}</p>
      <div className="flex items-center justify-between">
        <Label>{t("enableAutoReply")}</Label>
        <Switch checked={replyEnabled} onCheckedChange={setReplyEnabled} />
      </div>
      {replyEnabled && (
        <>
          <div className="space-y-2">
            <Label>{t("subject")}</Label>
            <Input value={replySubject} onChange={(e) => setReplySubject(e.target.value)} placeholder="Re: away" />
          </div>
          <div className="space-y-2">
            <Label>{t("body")}</Label>
            <textarea
              value={replyBody}
              onChange={(e) => setReplyBody(e.target.value)}
              rows={4}
              className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm"
            />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>{t("startDate")}</Label>
              <Input type="date" value={replyStartdate} onChange={(e) => setReplyStartdate(e.target.value)} />
            </div>
            <div className="space-y-2">
              <Label>{t("endDate")}</Label>
              <Input type="date" value={replyEnddate} onChange={(e) => setReplyEnddate(e.target.value)} />
            </div>
          </div>
        </>
      )}
    </div>
  );
}

export function SpamSection({
  t,
  spamEnabled, setSpamEnabled,
  spamMarkAsRead, setSpamMarkAsRead,
  spamThreshold, setSpamThreshold,
}: {
  t: (key: string) => string;
  spamEnabled: boolean; setSpamEnabled: (v: boolean) => void;
  spamMarkAsRead: boolean; setSpamMarkAsRead: (v: boolean) => void;
  spamThreshold: number; setSpamThreshold: (v: number) => void;
}) {
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">{t("spamFilter")}</p>
      <div className="flex items-center justify-between">
        <Label>{t("enableSpam")}</Label>
        <Switch checked={spamEnabled} onCheckedChange={setSpamEnabled} />
      </div>
      {spamEnabled && (
        <>
          <div className="flex items-center justify-between">
            <Label>{t("markAsRead")}</Label>
            <Switch checked={spamMarkAsRead} onCheckedChange={setSpamMarkAsRead} />
          </div>
          <div className="space-y-2">
            <Label>{t("threshold")}</Label>
            <Input type="number" min={0} max={100} value={spamThreshold} onChange={(e) => setSpamThreshold(Number(e.target.value))} />
          </div>
        </>
      )}
    </div>
  );
}

export function FiltersSection({
  t,
  whitelist, setWhitelist,
  blacklist, setBlacklist,
}: {
  t: (key: string) => string;
  whitelist: string; setWhitelist: (v: string) => void;
  blacklist: string; setBlacklist: (v: string) => void;
}) {
  return (
    <div className="space-y-5">
      <p className="text-sm font-medium">{t("filters")}</p>
      <div className="space-y-2">
        <Label>{t("whitelist")}</Label>
        <textarea
          value={whitelist}
          onChange={(e) => setWhitelist(e.target.value)}
          rows={2}
          className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm"
        />
        <p className="text-xs text-muted-foreground">{t("whitelistHint")}</p>
      </div>
      <div className="space-y-2">
        <Label>{t("blacklist")}</Label>
        <textarea
          value={blacklist}
          onChange={(e) => setBlacklist(e.target.value)}
          rows={2}
          className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm"
        />
        <p className="text-xs text-muted-foreground">{t("blacklistHint")}</p>
      </div>
    </div>
  );
}
