import {describe, expect, it} from "vitest";
import {Editor} from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";

function makeEditor(content: string, onUpdate: () => void) {
  return new Editor({
    element: document.createElement("div"),
    extensions: [StarterKit],
    content,
    onUpdate,
  });
}

describe("TipTap mount/normalization behavior", () => {
  it("does not emit onUpdate on initial content", async () => {
    const calls: string[] = [];
    const editor = makeEditor("<p>a</p>", () => calls.push("update"));
    await new Promise((r) => setTimeout(r, 50));
    expect(calls).toEqual([]);
    editor.destroy();
  });

  it("does not emit onUpdate when setContent is called with emitUpdate false", async () => {
    const calls: string[] = [];
    const editor = makeEditor("<p>a</p>", () => calls.push("update"));
    editor.commands.setContent("<p>b<br>c</p>", {emitUpdate: false});
    await new Promise((r) => setTimeout(r, 50));
    expect(calls).toEqual([]);
    editor.destroy();
  });

  it("emits onUpdate when setContent is called with the default emitUpdate", async () => {
    const calls: string[] = [];
    const editor = makeEditor("<p>a</p>", () => calls.push("update"));
    editor.commands.setContent("<p>b</p>");
    await new Promise((r) => setTimeout(r, 50));
    expect(calls).toEqual(["update"]);
    editor.destroy();
  });

  it("normalizes the document HTML on getHTML", () => {
    const editor = makeEditor("<p>a</p>", () => {});
    const html = editor.getHTML();
    expect(html).toContain("<p>a</p>");
    editor.destroy();
  });
});
