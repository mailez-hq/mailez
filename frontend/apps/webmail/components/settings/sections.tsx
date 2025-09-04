"use client";

import { Button } from "@/components/ui/button";
import {
  type AppToken, type AppTokenResult, type DelegationListing, type MailAccount, type MailDelegation, type MeSettings, type PgpKey, type PgpStatus, type SmimeCert, type SmimeStatus,
  type TotpStatus, type Webhook,
} from "@/lib/api";
import type { Accent, Density, Landing, Preferences, ReaderFontSize, ReadingPaneWidth, Theme } from "@/lib/preferences";
import { AppearanceSection } from "./sections/appearance-section";
import { AccountsSection } from "./sections/accounts-section";
import { CalendarSyncSection } from "./sections/calendar-sync-section";
import { DelegationsSection } from "@/modules/delegations-section";
import { PasswordSection } from "./sections/password-section";
import { PgpSection } from "./sections/pgp-section";
import {
  AiSection, AutoReplySection, FiltersSection, ForwardingSection, IdentitySection, NotificationsSection, SpamSection,
} from "./sections/profile-sections";
import { SmimeSection } from "@/modules/smime-section";
import { TwoFactorSection } from "./sections/two-factor-section";
import { WebhooksSection } from "./sections/webhooks-section";

// Sections that share the single profile <form> and its save button; the
// PGP / 2FA / password sections manage their own state and actions.
const PROFILE_SECTIONS = new Set([
  "appearance", "ai", "notifications", "identity",
  "forwarding", "autoReply", "spam", "filters",
]);

export type SettingsSectionsProps = {
  t: (key: string) => string;
  section: string;
  profile: MeSettings | null;
  theme: Theme; setTheme: (v: Theme) => void;
  density: Density; setDensity: (v: Density) => void;
  accent: Accent; setAccent: (v: Accent) => void;
  spellcheck: boolean; setSpellcheck: (v: boolean) => void;
  readerFont: ReaderFontSize; setReaderFont: (v: ReaderFontSize) => void;
  paneWidth: ReadingPaneWidth; setPaneWidth: (v: ReadingPaneWidth) => void;
  conversation: boolean; setConversation: (v: boolean) => void;
  autoSignature: boolean; setAutoSignature: (v: boolean) => void;
  collapseReplyQuote: boolean; setCollapseReplyQuote: (v: boolean) => void;
  landing: Landing; setLanding: (v: Landing) => void;
  prefs: Preferences; setAi: (ai: Preferences["ai"]) => void; setNotifications: (v: boolean) => void;
  displayedName: string; setDisplayedName: (v: string) => void;
  signature: string; setSignature: (v: string) => void;
  whitelist: string; setWhitelist: (v: string) => void;
  blacklist: string; setBlacklist: (v: string) => void;
  forwardEnabled: boolean; setForwardEnabled: (v: boolean) => void;
  forwardDestination: string; setForwardDestination: (v: string) => void;
  forwardKeep: boolean; setForwardKeep: (v: boolean) => void;
  replyEnabled: boolean; setReplyEnabled: (v: boolean) => void;
  replySubject: string; setReplySubject: (v: string) => void;
  replyBody: string; setReplyBody: (v: string) => void;
  replyStartdate: string; setReplyStartdate: (v: string) => void;
  replyEnddate: string; setReplyEnddate: (v: string) => void;
  spamEnabled: boolean; setSpamEnabled: (v: boolean) => void;
  spamMarkAsRead: boolean; setSpamMarkAsRead: (v: boolean) => void;
  spamThreshold: number; setSpamThreshold: (v: number) => void;
  saveSettings: (e: React.FormEvent) => void;
  savePassword: (e: React.FormEvent) => void;
  pgp: PgpStatus | null; setPgp: (v: PgpStatus | null) => void;
  generating: boolean; setGenerating: (v: boolean) => void;
  copied: boolean; setCopied: (v: boolean) => void;
  pgpKeys: PgpKey[] | null;
  pgpImportEmail: string; setPgpImportEmail: (v: string) => void;
  pgpImportKeyText: string; setPgpImportKeyText: (v: string) => void;
  pgpImporting: boolean;
  pgpDeleting: number | null;
  onImportPgpKey: () => void;
  onDeletePgpKey: (id: number) => void;
  totp: TotpStatus | null; setTotp: (v: TotpStatus | null) => void;
  totpCode: string; setTotpCode: (v: string) => void;
  totpBusy: boolean; setTotpBusy: (v: boolean) => void;
  webhooks: Webhook[] | null;
  whUrl: string; setWhUrl: (v: string) => void;
  whSecret: string; setWhSecret: (v: string) => void;
  whEvents: string; setWhEvents: (v: string) => void;
  whEnabled: boolean; setWhEnabled: (v: boolean) => void;
  whSaving: boolean;
  whTesting: number | null;
  whTestMsg: string;
  onAddWebhook: () => void;
  onDeleteWebhook: (id: number) => void;
  onToggleWebhook: (w: Webhook) => void;
  onTestWebhook: (id: number) => void;
  smime: SmimeStatus | null;
  smimeCerts: SmimeCert[] | null;
  smimeImportMode: "pem" | "p12"; setSmimeImportMode: (v: "pem" | "p12") => void;
  smimeCertPem: string; setSmimeCertPem: (v: string) => void;
  smimeKeyPem: string; setSmimeKeyPem: (v: string) => void;
  smimeP12B64: string; setSmimeP12B64: (v: string) => void;
  smimeP12Password: string; setSmimeP12Password: (v: string) => void;
  smimeImporting: boolean;
  smimeKeyringEmail: string; setSmimeKeyringEmail: (v: string) => void;
  smimeKeyringCert: string; setSmimeKeyringCert: (v: string) => void;
  smimeKeyringImporting: boolean;
  smimeKeyringDeleting: number | null;
  onImportSmime: () => void;
  onDeleteSmime: () => void;
  onImportSmimeCert: () => void;
  onDeleteSmimeCert: (id: number) => void;
  accountList: MailAccount[] | null;
  accName: string; setAccName: (v: string) => void;
  accEmail: string; setAccEmail: (v: string) => void;
  accImapHost: string; setAccImapHost: (v: string) => void;
  accImapPort: number; setAccImapPort: (v: number) => void;
  accImapSecurity: string; setAccImapSecurity: (v: string) => void;
  accSmtpHost: string; setAccSmtpHost: (v: string) => void;
  accSmtpPort: number; setAccSmtpPort: (v: number) => void;
  accSmtpSecurity: string; setAccSmtpSecurity: (v: string) => void;
  accUsername: string; setAccUsername: (v: string) => void;
  accPassword: string; setAccPassword: (v: string) => void;
  accSaving: boolean;
  accTesting: number | null;
  accTestMsg: string;
  onAddAccount: () => void;
  editingAccount: MailAccount | null;
  onStartEditAccount: (a: MailAccount) => void;
  onCancelEditAccount: () => void;
  onDeleteAccount: (id: number) => void;
  onToggleAccount: (a: MailAccount) => void;
  onTestAccount: (id: number) => void;
  delegationList: DelegationListing | null;
  delEmail: string; setDelEmail: (v: string) => void;
  delCanSend: boolean; setDelCanSend: (v: boolean) => void;
  delFullAccess: boolean; setDelFullAccess: (v: boolean) => void;
  delSaving: boolean;
  onAddDelegation: () => void;
  onUpdateDelegation: (d: MailDelegation) => void;
  onDeleteDelegation: (id: number) => void;
  davTokens: AppToken[];
  davNewToken: AppTokenResult | null;
  davTokenBusy: boolean;
  onCreateDavToken: () => void;
  onDeleteDavToken: (id: number) => void;
  setError: (v: string) => void;
  oldPw: string; setOldPw: (v: string) => void;
  newPw: string; setNewPw: (v: string) => void;
  confirmPw: string; setConfirmPw: (v: string) => void;
};

// SettingsSections renders the active settings section content. The
// MailSettings shell owns all state and passes it down; each section lives
// in its own file under sections/ and declares only the props it uses.
// Note: accSmtpPort / setAccSmtpPort remain in the props type for
// MailSettings compatibility but no section renders an SMTP port field.
export function SettingsSections(props: SettingsSectionsProps) {
  const {
    t, section, profile,
    theme, setTheme, density, setDensity, accent, setAccent, spellcheck, setSpellcheck,
    readerFont, setReaderFont, paneWidth, setPaneWidth, conversation, setConversation,
    autoSignature, setAutoSignature, collapseReplyQuote, setCollapseReplyQuote, landing, setLanding,
    prefs, setAi, setNotifications,
    displayedName, setDisplayedName, signature, setSignature,
    whitelist, setWhitelist, blacklist, setBlacklist,
    forwardEnabled, setForwardEnabled, forwardDestination, setForwardDestination, forwardKeep, setForwardKeep,
    replyEnabled, setReplyEnabled, replySubject, setReplySubject, replyBody, setReplyBody,
    replyStartdate, setReplyStartdate, replyEnddate, setReplyEnddate,
    spamEnabled, setSpamEnabled, spamMarkAsRead, setSpamMarkAsRead, spamThreshold, setSpamThreshold,
    saveSettings, savePassword,
    pgp, setPgp, generating, setGenerating, copied, setCopied, pgpKeys,
    pgpImportEmail, setPgpImportEmail, pgpImportKeyText, setPgpImportKeyText, pgpImporting, pgpDeleting,
    onImportPgpKey, onDeletePgpKey,
    totp, setTotp, totpCode, setTotpCode, totpBusy, setTotpBusy,
    webhooks, whUrl, setWhUrl, whSecret, setWhSecret, whEvents, setWhEvents, whEnabled, setWhEnabled,
    whSaving, whTesting, whTestMsg, onAddWebhook, onDeleteWebhook, onToggleWebhook, onTestWebhook,
    smime, smimeCerts, smimeImportMode, setSmimeImportMode, smimeCertPem, setSmimeCertPem,
    smimeKeyPem, setSmimeKeyPem, smimeP12B64, setSmimeP12B64, smimeP12Password, setSmimeP12Password,
    smimeImporting, smimeKeyringEmail, setSmimeKeyringEmail, smimeKeyringCert, setSmimeKeyringCert,
    smimeKeyringImporting, smimeKeyringDeleting, onImportSmime, onDeleteSmime, onImportSmimeCert, onDeleteSmimeCert,
    accountList, accName, setAccName, accEmail, setAccEmail, accImapHost, setAccImapHost,
    accImapPort, setAccImapPort, accImapSecurity, setAccImapSecurity, accSmtpHost, setAccSmtpHost,
    accSmtpSecurity, setAccSmtpSecurity, accUsername, setAccUsername, accPassword, setAccPassword,
    accSaving, accTesting, accTestMsg, onAddAccount, editingAccount, onStartEditAccount, onCancelEditAccount,
    onDeleteAccount, onToggleAccount, onTestAccount,
    delegationList, delEmail, setDelEmail, delCanSend, setDelCanSend, delFullAccess, setDelFullAccess,
    delSaving, onAddDelegation, onUpdateDelegation, onDeleteDelegation,
    davTokens, davNewToken, davTokenBusy, onCreateDavToken, onDeleteDavToken,
    setError,
    oldPw, setOldPw, newPw, setNewPw, confirmPw, setConfirmPw,
  } = props;

  return (
    <div className="min-w-0 flex-1">
      {/* Calendar sync renders standalone: it is read-only plus token
          generation, so it neither shares the profile save bar nor belongs
          inside the PROFILE_SECTIONS form. (It previously sat inside the
          form block while "calendarSync" was not in PROFILE_SECTIONS, so
          the sidebar entry rendered a blank pane.) */}
      {section === "calendarSync" && (
        <div key={section} className="h-full overflow-y-auto p-5">
          <CalendarSyncSection
            t={t}
            profile={profile}
            davTokens={davTokens}
            davNewToken={davNewToken}
            davTokenBusy={davTokenBusy}
            onCreateDavToken={onCreateDavToken}
            onDeleteDavToken={onDeleteDavToken}
          />
        </div>
      )}

      {PROFILE_SECTIONS.has(section) && (
        <form onSubmit={saveSettings} className="flex h-full min-h-0 flex-col">
          <div key={section} className="min-h-0 flex-1 space-y-5 overflow-y-auto p-5">
            {section === "appearance" && (
              <AppearanceSection
                t={t}
                theme={theme} setTheme={setTheme}
                landing={landing} setLanding={setLanding}
                density={density} setDensity={setDensity}
                accent={accent} setAccent={setAccent}
                spellcheck={spellcheck} setSpellcheck={setSpellcheck}
                readerFont={readerFont} setReaderFont={setReaderFont}
                paneWidth={paneWidth} setPaneWidth={setPaneWidth}
                conversation={conversation} setConversation={setConversation}
                collapseReplyQuote={collapseReplyQuote} setCollapseReplyQuote={setCollapseReplyQuote}
              />
            )}

            {section === "ai" && <AiSection t={t} prefs={prefs} setAi={setAi} />}

            {section === "notifications" && (
              <NotificationsSection t={t} prefs={prefs} setNotifications={setNotifications} />
            )}

            {section === "identity" && (
              <IdentitySection
                t={t}
                profile={profile}
                displayedName={displayedName} setDisplayedName={setDisplayedName}
                signature={signature} setSignature={setSignature}
                autoSignature={autoSignature} setAutoSignature={setAutoSignature}
              />
            )}

            {section === "forwarding" && (
              <ForwardingSection
                t={t}
                forwardEnabled={forwardEnabled} setForwardEnabled={setForwardEnabled}
                forwardDestination={forwardDestination} setForwardDestination={setForwardDestination}
                forwardKeep={forwardKeep} setForwardKeep={setForwardKeep}
              />
            )}

            {section === "autoReply" && (
              <AutoReplySection
                t={t}
                replyEnabled={replyEnabled} setReplyEnabled={setReplyEnabled}
                replySubject={replySubject} setReplySubject={setReplySubject}
                replyBody={replyBody} setReplyBody={setReplyBody}
                replyStartdate={replyStartdate} setReplyStartdate={setReplyStartdate}
                replyEnddate={replyEnddate} setReplyEnddate={setReplyEnddate}
              />
            )}

            {section === "spam" && (
              <SpamSection
                t={t}
                spamEnabled={spamEnabled} setSpamEnabled={setSpamEnabled}
                spamMarkAsRead={spamMarkAsRead} setSpamMarkAsRead={setSpamMarkAsRead}
                spamThreshold={spamThreshold} setSpamThreshold={setSpamThreshold}
              />
            )}

            {section === "filters" && (
              <FiltersSection
                t={t}
                whitelist={whitelist} setWhitelist={setWhitelist}
                blacklist={blacklist} setBlacklist={setBlacklist}
              />
            )}
          </div>
          <div className="flex shrink-0 justify-end border-t bg-muted/40 px-5 py-3">
            <Button type="submit">{t("saveSettings")}</Button>
          </div>
        </form>
      )}

      {section === "accounts" && (
        <AccountsSection
          t={t}
          accountList={accountList}
          accName={accName} setAccName={setAccName}
          accEmail={accEmail} setAccEmail={setAccEmail}
          accImapHost={accImapHost} setAccImapHost={setAccImapHost}
          accImapPort={accImapPort} setAccImapPort={setAccImapPort}
          accImapSecurity={accImapSecurity} setAccImapSecurity={setAccImapSecurity}
          accSmtpHost={accSmtpHost} setAccSmtpHost={setAccSmtpHost}
          accSmtpSecurity={accSmtpSecurity} setAccSmtpSecurity={setAccSmtpSecurity}
          accUsername={accUsername} setAccUsername={setAccUsername}
          accPassword={accPassword} setAccPassword={setAccPassword}
          accSaving={accSaving}
          accTesting={accTesting}
          accTestMsg={accTestMsg}
          onAddAccount={onAddAccount}
          editingAccount={editingAccount}
          onStartEditAccount={onStartEditAccount}
          onCancelEditAccount={onCancelEditAccount}
          onDeleteAccount={onDeleteAccount}
          onToggleAccount={onToggleAccount}
          onTestAccount={onTestAccount}
        />
      )}

      {section === "delegations" && (
        <DelegationsSection
          t={t}
          delegationList={delegationList}
          delEmail={delEmail} setDelEmail={setDelEmail}
          delCanSend={delCanSend} setDelCanSend={setDelCanSend}
          delFullAccess={delFullAccess} setDelFullAccess={setDelFullAccess}
          delSaving={delSaving}
          onAddDelegation={onAddDelegation}
          onUpdateDelegation={onUpdateDelegation}
          onDeleteDelegation={onDeleteDelegation}
        />
      )}

      {section === "pgp" && (
        <PgpSection
          t={t}
          pgp={pgp} setPgp={setPgp}
          generating={generating} setGenerating={setGenerating}
          copied={copied} setCopied={setCopied}
          pgpKeys={pgpKeys}
          pgpImportEmail={pgpImportEmail} setPgpImportEmail={setPgpImportEmail}
          pgpImportKeyText={pgpImportKeyText} setPgpImportKeyText={setPgpImportKeyText}
          pgpImporting={pgpImporting}
          pgpDeleting={pgpDeleting}
          onImportPgpKey={onImportPgpKey}
          onDeletePgpKey={onDeletePgpKey}
          setError={setError}
        />
      )}

      {section === "smime" && (
        <SmimeSection
          t={t}
          smime={smime}
          smimeCerts={smimeCerts}
          smimeImportMode={smimeImportMode} setSmimeImportMode={setSmimeImportMode}
          smimeCertPem={smimeCertPem} setSmimeCertPem={setSmimeCertPem}
          smimeKeyPem={smimeKeyPem} setSmimeKeyPem={setSmimeKeyPem}
          smimeP12B64={smimeP12B64} setSmimeP12B64={setSmimeP12B64}
          smimeP12Password={smimeP12Password} setSmimeP12Password={setSmimeP12Password}
          smimeImporting={smimeImporting}
          smimeKeyringEmail={smimeKeyringEmail} setSmimeKeyringEmail={setSmimeKeyringEmail}
          smimeKeyringCert={smimeKeyringCert} setSmimeKeyringCert={setSmimeKeyringCert}
          smimeKeyringImporting={smimeKeyringImporting}
          smimeKeyringDeleting={smimeKeyringDeleting}
          onImportSmime={onImportSmime}
          onDeleteSmime={onDeleteSmime}
          onImportSmimeCert={onImportSmimeCert}
          onDeleteSmimeCert={onDeleteSmimeCert}
        />
      )}

      {section === "webhooks" && (
        <WebhooksSection
          t={t}
          webhooks={webhooks}
          whUrl={whUrl} setWhUrl={setWhUrl}
          whSecret={whSecret} setWhSecret={setWhSecret}
          whEvents={whEvents} setWhEvents={setWhEvents}
          whEnabled={whEnabled} setWhEnabled={setWhEnabled}
          whSaving={whSaving}
          whTesting={whTesting}
          whTestMsg={whTestMsg}
          onAddWebhook={onAddWebhook}
          onDeleteWebhook={onDeleteWebhook}
          onToggleWebhook={onToggleWebhook}
          onTestWebhook={onTestWebhook}
        />
      )}

      {section === "twoFactor" && (
        <TwoFactorSection
          t={t}
          totp={totp} setTotp={setTotp}
          totpCode={totpCode} setTotpCode={setTotpCode}
          totpBusy={totpBusy} setTotpBusy={setTotpBusy}
          setError={setError}
        />
      )}

      {section === "password" && (
        <PasswordSection
          t={t}
          oldPw={oldPw} setOldPw={setOldPw}
          newPw={newPw} setNewPw={setNewPw}
          confirmPw={confirmPw} setConfirmPw={setConfirmPw}
          savePassword={savePassword}
        />
      )}
    </div>
  );
}
