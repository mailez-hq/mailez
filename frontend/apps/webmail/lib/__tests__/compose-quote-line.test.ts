import {describe, expect, it} from "vitest";
import {Editor} from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";
import {textToHtml} from "../../components/mailbox/mail-utils";

// Builds a TipTap editor attached to a real DOM element, like the compose
// editor, and runs the same "cursor line above the quote" insertion the
// component performs on reply/forward.
function replyEditor() {
  const el = document.createElement("div");
  document.body.appendChild(el);
  const editor = new Editor({
    element: el,
    extensions: [StarterKit],
    content: textToHtml("\n\nOn 2026-08-26, admin@example.com wrote:\n> hello"),
  });
  const first = editor.state.doc.firstChild;
  const startsEmpty = !!first && first.isTextblock && first.content.size === 0;
  if (!startsEmpty) {
    editor.commands.insertContentAt(0, {type: "paragraph"}, {updateSelection: true});
  }
  editor.commands.focus("start");
  return {editor, el};
}

describe("compose editor cursor line above a quote", () => {
  it("inserts exactly one blank line, not a two-line hard break", () => {
    const {editor, el} = replyEditor();
    try {
      const first = editor.state.doc.firstChild!;
      expect(first.isTextblock).toBe(true);
      // The inserted paragraph is genuinely empty: a textblock whose content
      // size is 0 renders as one blank line. Inserting "<p><br></p>" would
      // parse the <br> as a hard break and render TWO blank lines.
      expect(first.content.size).toBe(0);
      const firstP = el.querySelector("p")!;
      expect(firstP.querySelectorAll("br").length).toBe(1);
      // The quote follows right below the single blank line.
      expect(editor.getHTML()).toMatch(/^<p><\/p><p>On 2026-08-26/);
    } finally {
      editor.destroy();
      el.remove();
    }
  });
});
