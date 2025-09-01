"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import {
  ChevronRight, Copy, Download, File as FileIcon, Folder, FolderPlus,
  Link2, Loader2, MoreHorizontal, Pencil, RefreshCw, Trash2, Upload,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  ApiError,
  driveCreateFolder, driveDownload, driveEmptyTrash, driveMove, driveRename,
  driveRestore, driveShare, driveShareRevoke, driveTrash, driveTrashEntries, driveTree,
  driveUpload, type DriveEntry,
} from "@/lib/api";
import { cn } from "@/lib/utils";

// folderChainTo walks the tree from the root and returns the folder chain
// ending at folderId (BFS via one driveTree call per level; personal drives
// are shallow). Empty chain = folder sits at the root (or was not found).
async function folderChainTo(folderId: number): Promise<DriveEntry[]> {
  if (folderId === 0) return [];
  const queue: DriveEntry[][] = [[]];
  const visited = new Set<number>();
  while (queue.length > 0) {
    const chain = queue.shift()!;
    const parentId = chain.length > 0 ? chain[chain.length - 1].id : 0;
    let children: DriveEntry[];
    try {
      children = await driveTree(parentId);
    } catch {
      return [];
    }
    for (const c of children) {
      if (!c.is_dir || visited.has(c.id)) continue;
      visited.add(c.id);
      const next = [...chain, c];
      if (c.id === folderId) return next;
      queue.push(next);
    }
  }
  return [];
}

function fmtSize(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

// DriveView is the file manager body: breadcrumb navigation, upload/folder
// creation, per-entry download/share/rename/move/trash and a trash bucket.
// `focus` (recent-files card) navigates to the file's folder on mount/change
// and briefly highlights the row.
export function DriveView({ focus }: { focus?: DriveEntry | null }) {
  const t = useTranslations("drive");
  const [cwd, setCwd] = useState<DriveEntry[]>([]); // breadcrumb chain
  const [entries, setEntries] = useState<DriveEntry[]>([]);
  const [trash, setTrash] = useState<DriveEntry[]>([]);
  const [showTrash, setShowTrash] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [newFolder, setNewFolder] = useState("");
  const [folderOpen, setFolderOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const [menu, setMenu] = useState<{ x: number; y: number; entry: DriveEntry } | null>(null);
  const [renaming, setRenaming] = useState<DriveEntry | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [copied, setCopied] = useState(false);
  const [highlightId, setHighlightId] = useState<number | null>(null);

  const parentId = cwd.length > 0 ? cwd[cwd.length - 1].id : 0;

  // Show the spinner synchronously on folder switches: the render-phase
  // adjustment reacts one render earlier than an effect would.
  const [prevParentId, setPrevParentId] = useState(parentId);
  if (parentId !== prevParentId) {
    setPrevParentId(parentId);
    setLoading(true);
    setError("");
  }

  const load = useCallback(async () => {
    try {
      const [list, trashList] = await Promise.all([driveTree(parentId), driveTrash()]);
      setEntries(list);
      setTrash(trashList);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoading(false);
    }
  }, [parentId]);

  useEffect(() => {
    // Fetch whenever the folder changes. Every setState inside load() happens
    // after its await — the lint cannot see through the call boundary.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load();
  }, [load]);

  // Focus navigation (workspace recent-files click): resolve the file's
  // folder chain, navigate there, and flash the row so it's easy to spot.
  // The regular load effect follows the cwd change; if the file already sits
  // in the open folder nothing reloads and only the highlight applies.
  useEffect(() => {
    if (!focus) return;
    let cancelled = false;
    setHighlightId(focus.id);
    (async () => {
      setLoading(true);
      const chain = await folderChainTo(focus.parent_id);
      if (cancelled) return;
      setCwd(chain);
    })();
    const timer = setTimeout(() => setHighlightId(null), 4000);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [focus]);

  // Auto-clear the highlight even if the effect above is torn down early.
  useEffect(() => {
    if (highlightId === null) return;
    const timer = setTimeout(() => setHighlightId(null), 4000);
    return () => clearTimeout(timer);
  }, [highlightId]);

  const openFolder = (entry: DriveEntry) => {
    setCwd((chain) => [...chain, entry]);
  };

  const navTo = (index: number) => {
    setCwd((chain) => chain.slice(0, index));
  };

  const createFolder = async () => {
    if (!newFolder.trim()) return;
    setBusy(true);
    setError("");
    try {
      await driveCreateFolder(parentId, newFolder.trim());
      setNewFolder("");
      setFolderOpen(false);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "create failed");
    } finally {
      setBusy(false);
    }
  };

  const upload = async (files: FileList | null) => {
    if (!files || files.length === 0) return;
    setBusy(true);
    setError("");
    try {
      for (const f of Array.from(files)) {
        await driveUpload(f, parentId);
      }
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "upload failed");
    } finally {
      setBusy(false);
    }
  };

  const download = async (entry: DriveEntry) => {
    try {
      const blob = await driveDownload(entry.id);
      const a = document.createElement("a");
      a.href = URL.createObjectURL(blob);
      a.download = entry.name;
      a.click();
      URL.revokeObjectURL(a.href);
    } catch {
      setError(t("downloadFailed"));
    }
  };

  const share = async (entry: DriveEntry) => {
    try {
      const res = await driveShare(entry.id);
      await navigator.clipboard.writeText(res.url);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
      setMenu(null);
    } catch (e) {
      // The community edition caps active share links; surface friendly copy.
      setError(
        e instanceof ApiError && e.code === "quota_exceeded"
          ? t("shareQuotaReached")
          : t("shareFailed"),
      );
    }
  };

  const revokeShare = async (entry: DriveEntry) => {
    try {
      await driveShareRevoke(entry.id);
      setMenu(null);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("revokeFailed"));
    }
  };

  const rename = async () => {
    if (!renaming || !renameValue.trim()) return;
    try {
      await driveRename(renaming.id, renameValue.trim());
      setRenaming(null);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "rename failed");
    }
  };

  const trashEntries = async (entry: DriveEntry) => {
    try {
      await driveTrashEntries([entry.id]);
      setMenu(null);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "trash failed");
    }
  };

  const restore = async (entry: DriveEntry) => {
    try {
      await driveRestore([entry.id]);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "restore failed");
    }
  };

  const emptyTrash = async () => {
    try {
      await driveEmptyTrash();
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "empty failed");
    }
  };

  const moveTo = async (entry: DriveEntry, destination: number) => {
    try {
      await driveMove(entry.id, destination);
      setMenu(null);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "move failed");
    }
  };

  const renderEntry = (entry: DriveEntry, isTrash: boolean) => (
    <div
      key={entry.id}
      className={cn(
        "group flex items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-muted/60",
        highlightId === entry.id && "bg-primary/5 ring-1 ring-primary/50",
      )}
      onDoubleClick={() => entry.is_dir && !isTrash && openFolder(entry)}
    >
      {entry.is_dir ? (
        <Folder className="size-4 shrink-0 text-accent" />
      ) : (
        <FileIcon className="size-4 shrink-0 text-muted-foreground" />
      )}
      {renaming?.id === entry.id ? (
        <Input
          autoFocus
          value={renameValue}
          onChange={(e) => setRenameValue(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") rename();
            if (e.key === "Escape") setRenaming(null);
          }}
          className="h-6 flex-1 text-xs"
        />
      ) : (
        <button
          type="button"
          className="min-w-0 flex-1 truncate text-left"
          onClick={() => entry.is_dir && !isTrash && openFolder(entry)}
          title={entry.name}
        >
          {entry.name}
        </button>
      )}
      {!entry.is_dir && (
        <span className="shrink-0 text-[11px] text-muted-foreground">{fmtSize(entry.size)}</span>
      )}
      <div className="flex shrink-0 items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
        {!isTrash && !entry.is_dir && (
          <Button variant="ghost" size="icon-sm" onClick={() => download(entry)} title={t("download")}>
            <Download className="size-3.5" />
          </Button>
        )}
        {!isTrash && (
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={(e) => {
              e.stopPropagation();
              setMenu({ x: e.clientX, y: e.clientY, entry });
            }}
            title={t("more")}
          >
            <MoreHorizontal className="size-3.5" />
          </Button>
        )}
        {isTrash && (
          <Button variant="ghost" size="icon-sm" onClick={() => restore(entry)} title={t("restore")}>
            <RefreshCw className="size-3.5" />
          </Button>
        )}
      </div>
    </div>
  );

  return (
    <div className="flex h-full min-h-0 flex-col bg-background">
      <div className="flex shrink-0 items-center justify-between gap-2 border-b px-3 py-2">
        <div className="flex min-w-0 items-center gap-0.5 text-sm">
          <button type="button" className="font-medium hover:text-accent" onClick={() => navTo(0)}>
            {t("root")}
          </button>
          {cwd.map((d, i) => (
            <span key={d.id} className="flex min-w-0 items-center gap-0.5">
              <ChevronRight className="size-3 shrink-0 text-muted-foreground" />
              <button
                type="button"
                className="truncate hover:text-accent"
                onClick={() => navTo(i + 1)}
              >
                {d.name}
              </button>
            </span>
          ))}
        </div>
        <Button
          variant={showTrash ? "default" : "outline"}
          size="sm"
          onClick={() => setShowTrash((v) => !v)}
          title={t("trash")}
        >
          <Trash2 className="size-3.5" />
          <span className="hidden sm:inline">{t("trash")}</span>
        </Button>
      </div>
      <div className="flex shrink-0 items-center gap-2 border-b px-3 py-2">
        <Button size="sm" onClick={() => fileRef.current?.click()} disabled={busy}>
          {busy ? <Loader2 className="size-3.5 animate-spin" /> : <Upload className="size-3.5" />}
          {t("upload")}
        </Button>
        <input
          ref={fileRef}
          type="file"
          multiple
          className="hidden"
          onChange={(e) => upload(e.target.files)}
        />
        {!showTrash && (
          <Button size="sm" variant="outline" onClick={() => setFolderOpen((v) => !v)}>
            <FolderPlus className="size-3.5" />
            {t("newFolder")}
          </Button>
        )}
        {showTrash && trash.length > 0 && (
          <Button size="sm" variant="ghost" className="text-destructive" onClick={emptyTrash}>
            {t("emptyTrash")}
          </Button>
        )}
      </div>
      {folderOpen && !showTrash && (
        <div className="flex shrink-0 items-center gap-2 border-b px-3 py-2">
          <Input
            autoFocus
            value={newFolder}
            onChange={(e) => setNewFolder(e.target.value)}
            placeholder={t("folderName")}
            onKeyDown={(e) => e.key === "Enter" && createFolder()}
            className="h-7 max-w-56 text-xs"
          />
          <Button size="sm" onClick={createFolder} disabled={busy || !newFolder.trim()}>
            {t("create")}
          </Button>
        </div>
      )}
      {error && <p className="shrink-0 px-3 py-1 text-xs text-destructive">{error}</p>}
      <div className="min-h-0 flex-1 overflow-y-auto p-2">
        {loading ? (
          <p className="flex items-center gap-1.5 px-2 py-4 text-sm text-muted-foreground">
            <Loader2 className="size-3.5 animate-spin" />
            {t("loading")}
          </p>
        ) : showTrash ? (
          trash.length === 0 ? (
            <p className="px-2 py-4 text-sm text-muted-foreground">{t("trashEmpty")}</p>
          ) : (
            trash.map((e) => renderEntry(e, true))
          )
        ) : entries.length === 0 ? (
          <p className="px-2 py-4 text-sm text-muted-foreground">{t("empty")}</p>
        ) : (
          entries.map((e) => renderEntry(e, false))
        )}
      </div>
      {menu && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setMenu(null)} />
          <div
            className="fixed z-50 w-44 rounded-lg border border-border bg-popover p-1 text-sm shadow-lg"
            style={{ left: menu.x, top: menu.y }}
          >
            {!menu.entry.is_dir && (
              <button
                type="button"
                className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left hover:bg-muted"
                onClick={() => download(menu.entry)}
              >
                <Download className="size-3.5" />
                {t("download")}
              </button>
            )}
            {!menu.entry.is_dir && (
              <button
                type="button"
                className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left hover:bg-muted"
                onClick={() => share(menu.entry)}
              >
                {copied ? <Copy className="size-3.5" /> : <Link2 className="size-3.5" />}
                {copied ? t("copied") : menu.entry.share_token ? t("copyLink") : t("share")}
              </button>
            )}
            {!menu.entry.is_dir && menu.entry.share_token && (
              <button
                type="button"
                className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-destructive hover:bg-muted"
                onClick={() => revokeShare(menu.entry)}
              >
                <Link2 className="size-3.5" />
                {t("revokeShare")}
              </button>
            )}
            <button
              type="button"
              className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left hover:bg-muted"
              onClick={() => {
                setRenaming(menu.entry);
                setRenameValue(menu.entry.name);
                setMenu(null);
              }}
            >
              <Pencil className="size-3.5" />
              {t("rename")}
            </button>
            <button
              type="button"
              className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-destructive hover:bg-muted"
              onClick={() => trashEntries(menu.entry)}
            >
              <Trash2 className="size-3.5" />
              {t("trash")}
            </button>
            {cwd.length > 0 && (
              <button
                type="button"
                className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left hover:bg-muted"
                onClick={() => moveTo(menu.entry, cwd.length > 1 ? cwd[cwd.length - 2].id : 0)}
              >
                <ChevronRight className="size-3.5" />
                {t("moveUp")}
              </button>
            )}
          </div>
        </>
      )}
      {copied && (
        <p className="shrink-0 border-t px-3 py-1 text-[11px] text-muted-foreground">{t("linkCopied")}</p>
      )}
    </div>
  );
}
