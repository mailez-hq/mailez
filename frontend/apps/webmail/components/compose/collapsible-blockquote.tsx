"use client";

import type { NodeViewProps } from "@tiptap/react";
import { NodeViewContent, NodeViewWrapper, ReactNodeViewRenderer } from "@tiptap/react";
import Blockquote from "@tiptap/extension-blockquote";
import { mergeAttributes } from "@tiptap/core";
import { useTranslations } from "next-intl";

import { cn } from "@/lib/utils";

// Gmail-style quote in the compose editor: reply quotes render collapsed as a
// single "…" row by default and expand on click, while the quoted text stays
// in the document model so the sent message (and the plain-text body) still
// carries the full quote. The collapsed state lives in a data-collapsed
// attribute, which is stripped from the wire body on send but kept in drafts
// so reopening a draft remembers whether the quote was expanded.
function BlockquoteView({ node, updateAttributes, editor, getPos }: NodeViewProps) {
  const collapsed = !!node.attrs.collapsed;
  const t = useTranslations("mail");

  const toggle = (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const next = !collapsed;
    updateAttributes({ collapsed: next });
    if (next) {
      // The quote is about to become display:none; move the caret to the end
      // of the paragraph above so it never lands inside hidden content.
      const pos = getPos();
      if (typeof pos === "number" && pos > 0) {
        editor.chain().focus().setTextSelection(Math.max(0, pos - 1)).run();
      }
    }
  };

  return (
    <NodeViewWrapper as="div">
      <blockquote
        data-collapsed={collapsed || undefined}
        className={cn(
          "my-1 rounded-r-md text-muted-foreground",
          collapsed ? "border-l-0 pl-0" : "border-l-2 border-border pl-3",
        )}
      >
        <NodeViewContent
          as="div"
          className={cn(collapsed && "hidden")}
          style={{ whiteSpace: "normal" }}
        />
        <button
          type="button"
          contentEditable={false}
          onClick={toggle}
          title={collapsed ? t("expandQuote") : t("collapseQuote")}
          className={cn(
            "text-xs text-muted-foreground hover:text-foreground",
            collapsed
              ? "cursor-pointer rounded px-1.5 py-0.5 hover:bg-muted"
              : "mt-1 block px-1",
          )}
        >
          …
        </button>
      </blockquote>
    </NodeViewWrapper>
  );
}

export const CollapsibleBlockquote = Blockquote.extend({
  addAttributes() {
    return {
      collapsed: {
        default: false,
        parseHTML: (el) => el.dataset.collapsed === "true",
        renderHTML: (attrs) => (attrs.collapsed ? { "data-collapsed": "true" } : {}),
      },
    };
  },
  renderHTML({ HTMLAttributes }) {
    return ["blockquote", mergeAttributes(this.options.HTMLAttributes, HTMLAttributes), 0];
  },
  addNodeView() {
    return ReactNodeViewRenderer(BlockquoteView);
  },
});
