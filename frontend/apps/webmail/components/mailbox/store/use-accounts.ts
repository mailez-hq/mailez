import { useCallback, useEffect, useRef, useState } from "react";

import {
  accounts,
  delegations,
  setActiveAccountId,
  setActiveDelegateEmail,
  type MailAccount,
  type MailDelegation,
} from "@/lib/api";

import { detailCache, threadCache } from "./view-caches";

type ResetView = (opts: { includeLabel?: boolean }) => void;

/**
 * Aggregated external accounts and delegated (shared) mailboxes. Switching
 * either re-scopes every /mail/* request (active account id or
 * X-Delegate-Email header) and resets the mailbox view via `resetView`.
 */
export function useAccounts(
  router: { push: (href: string) => void },
  resetView: ResetView,
  loadFolders: () => void | Promise<void>,
  loadMessages: (folder: string, page?: number, silent?: boolean) => Promise<void>,
) {
  // ---- aggregated external accounts (full aggregation client) ----
  const [accountList, setAccountList] = useState<MailAccount[]>([]);
  // null = the internal gateway account; a number = an external mailbox.
  const [activeAccount, setActiveAccount] = useState<number | null>(null);
  // ---- delegated mailboxes (shared mailboxes with full access) ----
  // delegateList holds every owner mailbox this user may open; activeDelegate
  // is the currently open owner email (null = the user's own mailbox).
  const [delegateList, setDelegateList] = useState<MailDelegation[]>([]);
  const [activeDelegate, setActiveDelegate] = useState<string | null>(null);
  const switchInFlight = useRef(false);

  const refreshAccounts = useCallback(async () => {
    try {
      setAccountList(await accounts());
    } catch {
      // account listing is optional; the internal account always works
    }
  }, []);

  const refreshDelegations = useCallback(async () => {
    try {
      const listing = await delegations();
      setDelegateList(listing.received.filter((d) => d.full_access));
    } catch {
      // delegation listing is optional; the user's own mailbox always works
    }
  }, []);

  // Load the account list once; keep the active account in the api module so
  // every /mail/* request is scoped to it. Both loaders resolve every setState
  // after their await — the lint cannot see through the call boundary.
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    refreshAccounts();
  }, [refreshAccounts]);
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    refreshDelegations();
  }, [refreshDelegations]);
  useEffect(() => {
    setActiveAccountId(activeAccount);
  }, [activeAccount]);
  useEffect(() => {
    setActiveDelegateEmail(activeDelegate);
  }, [activeDelegate]);

  // Switching accounts resets the mailbox view and re-scopes all requests.
  async function switchAccount(id: number | null) {
    if (id === activeAccount || switchInFlight.current) return;
    switchInFlight.current = true;
    // The view caches are keyed by folder/uid only — without a flush the
    // next account would serve the previous mailbox's details.
    detailCache.clear();
    threadCache.clear();
    setActiveAccount(id);
    setActiveDelegate(null);
    resetView({});
    switchInFlight.current = false;
    router.push("/mail/Inbox");
    loadFolders();
    // resetView keeps folder at "Inbox": when the user was already there the
    // folder effect's dependencies do not change and would never reload, so
    // the list would stay empty. Load the scoped inbox explicitly.
    loadMessages("Inbox");
  }

  // Switching delegated mailboxes re-scopes every /mail/* request to the
  // owner's mailbox (X-Delegate-Email); the backend validates the grant.
  async function switchDelegate(email: string | null) {
    if (email === activeDelegate || switchInFlight.current) return;
    switchInFlight.current = true;
    detailCache.clear();
    threadCache.clear();
    setActiveAccount(null);
    setActiveDelegate(email);
    resetView({ includeLabel: true });
    switchInFlight.current = false;
    router.push("/mail/Inbox");
    loadFolders();
    loadMessages("Inbox");
  }

  return {
    accountList,
    activeAccount,
    refreshAccounts,
    delegateList,
    activeDelegate,
    switchAccount,
    switchDelegate,
    refreshDelegations,
  };
}

export type Accounts = ReturnType<typeof useAccounts>;
