// A folder-management dialog: create/rename take a name input, delete/clear ask
// for confirmation.
export type FolderDialog =
  | { mode: "create"; value: string }
  | { mode: "rename"; name: string; value: string }
  | { mode: "delete"; name: string }
  | { mode: "clear"; name: string };
