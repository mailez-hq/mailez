import { describe, expect, it } from "vitest";

import { buildFolderTree, flattenTree, sortFolders } from "@/components/mailbox/folder-tree";

describe("folder tree", () => {
  it("sorts common folders first with INBOX pinned", () => {
    expect(sortFolders(["Trash", "Inbox", "Projects", "Sent", "Inbox/Sub"])).toEqual([
      "Inbox",
      "Sent",
      "Trash",
      "Inbox/Sub",
      "Projects",
    ]);
  });

  it("nests subfolders under their parent instead of a flat duplicate", () => {
    const tree = buildFolderTree(["Inbox", "Inbox/helloworld", "Projects", "Projects/Invoice"]);
    expect(tree.map((n) => n.label)).toEqual(["Inbox", "Projects"]);
    expect(tree[0].children.map((n) => n.label)).toEqual(["helloworld"]);
    expect(tree[1].children.map((n) => n.label)).toEqual(["Invoice"]);
  });

  it("flattens the tree into value/label options with indentation", () => {
    const options = flattenTree(buildFolderTree(["Inbox", "Inbox/helloworld"]), (leaf) => leaf);
    expect(options).toEqual([
      { value: "Inbox", label: "Inbox" },
      { value: "Inbox/helloworld", label: "\u00A0\u00A0helloworld" },
    ]);
  });
});
