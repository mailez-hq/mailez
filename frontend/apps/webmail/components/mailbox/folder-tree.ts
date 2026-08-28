import type { useTranslations } from "next-intl";

// Folder hierarchy helpers shared by the sidebar and every move-to dropdown.
// IMAP expresses nesting with "/" as the separator; all surfaces must render
// the same sorted, indented tree so "Inbox/Sub" never shows as a flat entry.

// Common folders always sort before custom ones; INBOX is pinned first.
export const FOLDER_ORDER = ["INBOX", "Sent", "Drafts", "Trash", "Archive", "Junk", "Spam"];

// System mailboxes as created by the engine. Only INBOX is matched
// case-insensitively; a user-created folder that merely shares a system name
// in another case ("sent", "trash", ...) is a normal, manageable folder.
export const SYSTEM_FOLDERS = new Set(["Inbox", "Sent", "Drafts", "Trash", "Archive", "Junk"]);

export function isSystemFolder(name: string): boolean {
  if (name.includes("/")) return false;
  const head = name.toUpperCase() === "INBOX" ? "Inbox" : name;
  return SYSTEM_FOLDERS.has(head);
}

export function sortFolders(folders: string[]): string[] {
  // Compare case-insensitively: FOLDER_ORDER is Title-case ("Sent") while a
  // raw folder name may arrive as "SENT" or "sEnt".
  const rank = (f: string) => {
    const i = FOLDER_ORDER.findIndex((o) => o.toUpperCase() === f.toUpperCase());
    return i === -1 ? Number.MAX_SAFE_INTEGER : i;
  };
  return [...folders].sort((a, b) => {
    const ra = rank(a);
    const rb = rank(b);
    if (ra !== rb) return ra - rb;
    return a.localeCompare(b);
  });
}

export type FolderNode = {
  name: string; // full path, e.g. "Projects/Invoice"
  label: string; // last path segment, e.g. "Invoice"
  depth: number;
  children: FolderNode[];
};

// buildFolderTree groups the flat folder list into a nested tree by splitting
// each name on "/". Parent folders appear before their children (sorted by the
// existing folder ordering), so unknown intermediate parents get a node too.
export function buildFolderTree(folders: string[]): FolderNode[] {
  const roots: FolderNode[] = [];
  const byPath = new Map<string, FolderNode>();
  const ordered = sortFolders(folders);
  for (const f of ordered) {
    const parts = f.split("/");
    let parent: FolderNode | null = null;
    let path = "";
    for (let i = 0; i < parts.length; i++) {
      path = path ? `${path}/${parts[i]}` : parts[i];
      let node = byPath.get(path);
      if (!node) {
        node = { name: path, label: parts[i], depth: i, children: [] };
        byPath.set(path, node);
        if (parent) parent.children.push(node);
        else roots.push(node);
      }
      parent = node;
    }
  }
  return roots;
}

// folderLabel translates a known system folder (Inbox, Sent, ...) and falls
// back to the raw leaf name for custom folders.
export function folderLabel(t: ReturnType<typeof useTranslations<"mail">>, name: string) {
  const key = `folder${name.charAt(0).toUpperCase()}${name.slice(1).toLowerCase()}` as const;
  return t.has(key) ? t(key) : name;
}

// flattenTree walks the tree depth-first into a flat list of select options:
// the full path is the value, the indented translated leaf is the label.
export function flattenTree(
  nodes: FolderNode[],
  label: (leaf: string) => string,
): { value: string; label: string }[] {
  const out: { value: string; label: string }[] = [];
  const walk = (list: FolderNode[], depth: number) => {
    for (const n of list) {
      out.push({ value: n.name, label: `${"\u00A0".repeat(depth * 2)}${label(n.label)}` });
      walk(n.children, depth + 1);
    }
  };
  walk(nodes, 0);
  return out;
}
