"use client";

import { useCallback, useEffect, useMemo, useState, type Dispatch, type SetStateAction } from "react";

import { mailFolders, mailFolderClear, mailFolderCreate, mailFolderDelete, mailFolderRename, mailUnseen } from "@/lib/api";
import { readCachedFolders, writeCachedFolders } from "@/lib/folders-cache";

/** Translated labels for the folder-management toasts (next-intl "mail" ns). */
export type Translate = (key: string, values?: Record<string, string | number | Date>) => string;

/**
 * Folder cluster: the folder list and per-folder unseen counts, their load
 * paths, folder CRUD (create/rename/delete/clear) and the junk-folder probe.
 * The store passes `selectFolder` (which knows about active searches) for the
 * rename/delete redirect of the currently open folder.
 */
export function useFolderMgmt({
  email,
  folder,
  loadMessages,
  selectFolder,
  setError,
  showToast,
  t,
}: {
  email: string;
  folder: string;
  loadMessages: (folder: string, page?: number, silent?: boolean) => Promise<void>;
  selectFolder: (f: string) => void;
  setError: Dispatch<SetStateAction<string>>;
  showToast: (label: string, onUndo?: () => void, duration?: number) => void;
  t: Translate;
}) {
  const [folders, setFolders] = useState<string[]>([]);
  const [unseen, setUnseen] = useState<Record<string, number>>({});

  // Hydrate from the per-account cache so the sidebar paints populated on
  // the first frame (see lib/folders-cache.ts); the mount-time loadFolders
  // below revalidates. An effect, not a useState initializer, so the server
  // render and hydration stay identical (localStorage is client-only).
  useEffect(() => {
    const cached = readCachedFolders(email);
    if (cached?.length) setFolders(cached);
  }, [email]);

  const refreshUnseen = useCallback(() => {
    mailUnseen().then(setUnseen).catch(() => {});
  }, []);

  const loadFolders = useCallback(async () => {
    // Unseen counts don't depend on the folder list — fetch them in
    // parallel instead of after the folders response lands.
    refreshUnseen();
    try {
      const list = await mailFolders();
      writeCachedFolders(email, list);
      setFolders(list);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load folders failed");
    }
  }, [email, refreshUnseen, setError]);

  async function createFolder(name: string) {
    setError("");
    try {
      await mailFolderCreate(name);
      await loadFolders();
      showToast(t("toastFolderCreated", { name }));
    } catch (e) {
      setError(e instanceof Error ? e.message : "create folder failed");
    }
  }

  async function renameFolder(name: string, newName: string) {
    setError("");
    try {
      await mailFolderRename(name, newName);
      await loadFolders();
      if (folder.toLowerCase() === name.toLowerCase()) selectFolder(newName);
      showToast(t("toastFolderRenamed", { name }));
    } catch (e) {
      setError(e instanceof Error ? e.message : "rename folder failed");
    }
  }

  async function deleteFolder(name: string) {
    setError("");
    try {
      await mailFolderDelete(name);
      await loadFolders();
      if (folder.toLowerCase() === name.toLowerCase()) selectFolder("Inbox");
      showToast(t("toastFolderDeleted", { name }));
    } catch (e) {
      setError(e instanceof Error ? e.message : "delete folder failed");
    }
  }

  async function clearFolder(name: string) {
    setError("");
    try {
      await mailFolderClear(name);
      refreshUnseen();
      await loadMessages(folder);
      showToast(t("toastFolderCleared", { name }));
    } catch (e) {
      setError(e instanceof Error ? e.message : "clear folder failed");
    }
  }

  // Prefer the existing junk folder name (Spam in some setups) over
  // creating a duplicate Junk mailbox.
  const spamFolder = useMemo(() => {
    const hit = folders.find((f) => /^(junk|spam)$/i.test(f));
    return hit || "Junk";
  }, [folders]);

  return {
    folders, unseen, setUnseen,
    refreshUnseen, loadFolders,
    createFolder, renameFolder, deleteFolder, clearFolder,
    spamFolder,
  };
}
