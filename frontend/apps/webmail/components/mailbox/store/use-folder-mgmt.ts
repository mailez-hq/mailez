"use client";

import { useCallback, useMemo, useState, type Dispatch, type SetStateAction } from "react";

import { mailFolders, mailFolderClear, mailFolderCreate, mailFolderDelete, mailFolderRename, mailUnseen } from "@/lib/api";

/** Translated labels for the folder-management toasts (next-intl "mail" ns). */
export type Translate = (key: string, values?: Record<string, string | number | Date>) => string;

/**
 * Folder cluster: the folder list and per-folder unseen counts, their load
 * paths, folder CRUD (create/rename/delete/clear) and the junk-folder probe.
 * The store passes `selectFolder` (which knows about active searches) for the
 * rename/delete redirect of the currently open folder.
 */
export function useFolderMgmt({
  folder,
  loadMessages,
  selectFolder,
  setError,
  showToast,
  t,
}: {
  folder: string;
  loadMessages: (folder: string, page?: number, silent?: boolean) => Promise<void>;
  selectFolder: (f: string) => void;
  setError: Dispatch<SetStateAction<string>>;
  showToast: (label: string, onUndo?: () => void, duration?: number) => void;
  t: Translate;
}) {
  const [folders, setFolders] = useState<string[]>([]);
  const [unseen, setUnseen] = useState<Record<string, number>>({});

  const refreshUnseen = useCallback(() => {
    mailUnseen().then(setUnseen).catch(() => {});
  }, []);

  const loadFolders = useCallback(async () => {
    try {
      setFolders(await mailFolders());
    } catch (e) {
      setError(e instanceof Error ? e.message : "load folders failed");
    }
    refreshUnseen();
  }, [refreshUnseen, setError]);

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
