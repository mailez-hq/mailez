// Cloud drive API calls.
import {
  api,
  apiPost,
  mailPath,
  mailHeaders,
  handleUnauthorized,
  sessionExpiredText,
  API,
} from "./api-client";

// Cloud drive (云盘).
export type DriveEntry = {
  id: number;
  parent_id: number;
  name: string;
  is_dir: boolean;
  size: number;
  content_type: string;
  sha256: string;
  share_token: boolean;
  trashed: boolean;
  created_at: string;
  updated_at: string;
};

export const driveTree = (parentId = 0) =>
  api<DriveEntry[]>(`/drive/tree?parent_id=${parentId}`);

export const driveTrash = () => api<DriveEntry[]>("/drive/trash");

export const driveCreateFolder = (parentId: number, name: string) =>
  apiPost<DriveEntry>("/drive/folders", { parent_id: parentId, name });

export const driveUpload = (file: File, parentId: number) => {
  const fd = new FormData();
  fd.append("file", file);
  fd.append("parent_id", String(parentId));
  return fetch(`${API}${mailPath("/drive/upload")}`, {
    method: "POST",
    headers: mailHeaders(),
    body: fd,
  }).then(async (res) => {
    if (!res.ok) {
      if (res.status === 401) {
        handleUnauthorized("/drive/upload");
        throw new Error(sessionExpiredText());
      }
      const body = await res.json().catch(() => ({}));
      throw new Error((body as { error?: string }).error || "upload failed");
    }
    return (await res.json()) as DriveEntry;
  });
};

export async function driveDownload(id: number): Promise<Blob> {
  const res = await fetch(`${API}${mailPath(`/drive/download/${id}`)}`, {
    headers: mailHeaders(),
  });
  if (!res.ok) {
    if (res.status === 401) handleUnauthorized("/drive/download");
    throw new Error("download failed");
  }
  return res.blob();
}

export const driveShare = (id: number) =>
  api<{ url: string }>(`/drive/share/${id}`);

export const driveShareRevoke = (id: number) =>
  api<void>(`/drive/share/${id}`, { method: "DELETE" });

export const driveRename = (id: number, name: string) =>
  apiPost<void>("/drive/rename", { id, name });

export const driveMove = (id: number, parentId: number) =>
  apiPost<void>("/drive/move", { id, parent_id: parentId });

export const driveTrashEntries = (ids: number[]) =>
  apiPost<void>("/drive/trash", { ids });

export const driveRestore = (ids: number[]) =>
  apiPost<void>("/drive/restore", { ids });

export const driveEmptyTrash = () =>
  apiPost<void>("/drive/trash/empty", {});
