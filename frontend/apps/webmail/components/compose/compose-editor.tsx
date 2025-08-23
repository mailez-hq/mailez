"use client";

import { useEffect } from "react";
import { EditorContent, useEditor, useEditorState } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import Placeholder from "@tiptap/extension-placeholder";
import {
  Bold, Code2, Heading2, Heading3, Italic, Link as LinkIcon,
  List, ListOrdered, Quote, Redo2, Strikethrough, Underline, Undo2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

// Rich-text compose editor. Emits HTML for the wire and plain text as a
// degraded body for recipients that only understand text/plain.
export function ComposeEditor({ value, onChange, placeholder, autoFocus }: {
  value: string;
  onChange: (html: string, text: string) => void;
  placeholder?: string;
  // Focus the editor body once it mounts (used for reply/forward, where the
  // recipient is pre-filled and the user should start typing immediately).
  autoFocus?: boolean;
}) {
  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        heading: { levels: [2, 3] },
        link: { openOnClick: false },
      }),
      Placeholder.configure({ placeholder }),
    ],
    content: value || "",
    onUpdate: ({ editor }) => {
      const html = editor.isEmpty ? "" : editor.getHTML();
      onChange(html, editor.getText() || "");
    },
  });

  // keep the editor in sync when the value is replaced externally
  // (reply/forward quoting, AI drafts)
  useEffect(() => {
    if (!editor) return;
    const current = editor.isEmpty ? "" : editor.getHTML();
    if (value !== current) {
      editor.commands.setContent(value || "");
    }
  }, [value, editor]);

  // place the cursor above the quoted content on reply/forward so the user
  // can start typing right away; a short delay lets the dialog finish opening
  useEffect(() => {
    if (!editor || !autoFocus) return;
    const t = setTimeout(() => editor.commands.focus("start"), 60);
    return () => clearTimeout(t);
  }, [editor, autoFocus]);

  const state = useEditorState({
    editor,
    // the selector runs before the editor is created (and after it is
    // destroyed), so guard every access against a null instance
    selector: ({ editor }) => {
      const is = (name: string, attrs?: Record<string, unknown>) =>
        editor?.isActive(name, attrs) ?? false;
      return {
        bold: is("bold"),
        italic: is("italic"),
        underline: is("underline"),
        strike: is("strike"),
        h2: is("heading", { level: 2 }),
        h3: is("heading", { level: 3 }),
        bullet: is("bulletList"),
        ordered: is("orderedList"),
        quote: is("blockquote"),
        code: is("codeBlock"),
        link: is("link"),
      };
    },
  });

  if (!editor) {
    return <div className="min-h-[10rem] flex-1 rounded-md border" />;
  }

  function toggleLink() {
    if (state.link) {
      editor.chain().focus().unsetLink().run();
      return;
    }
    const href = window.prompt("Link URL", "https://");
    if (href) {
      editor.chain().focus().extendMarkRange("link").setLink({ href }).run();
    }
  }

  const toolBtn = (active: boolean, onClick: () => void, icon: React.ReactNode, title?: string) => (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      title={title}
      onClick={onClick}
      className={cn("h-7 w-7 p-0", active && "bg-accent text-accent-foreground")}
    >
      {icon}
    </Button>
  );

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-md border border-border">
      <div className="flex shrink-0 flex-wrap items-center gap-0.5 border-b bg-muted/40 px-2 py-1">
        {toolBtn(state.bold, () => editor.chain().focus().toggleBold().run(), <Bold className="h-3.5 w-3.5" />)}
        {toolBtn(state.italic, () => editor.chain().focus().toggleItalic().run(), <Italic className="h-3.5 w-3.5" />)}
        {toolBtn(state.underline, () => editor.chain().focus().toggleUnderline().run(), <Underline className="h-3.5 w-3.5" />)}
        {toolBtn(state.strike, () => editor.chain().focus().toggleStrike().run(), <Strikethrough className="h-3.5 w-3.5" />)}
        <span className="mx-1 h-4 w-px bg-border" />
        {toolBtn(state.h2, () => editor.chain().focus().toggleHeading({ level: 2 }).run(), <Heading2 className="h-3.5 w-3.5" />)}
        {toolBtn(state.h3, () => editor.chain().focus().toggleHeading({ level: 3 }).run(), <Heading3 className="h-3.5 w-3.5" />)}
        <span className="mx-1 h-4 w-px bg-border" />
        {toolBtn(state.bullet, () => editor.chain().focus().toggleBulletList().run(), <List className="h-3.5 w-3.5" />)}
        {toolBtn(state.ordered, () => editor.chain().focus().toggleOrderedList().run(), <ListOrdered className="h-3.5 w-3.5" />)}
        {toolBtn(state.quote, () => editor.chain().focus().toggleBlockquote().run(), <Quote className="h-3.5 w-3.5" />)}
        {toolBtn(state.code, () => editor.chain().focus().toggleCodeBlock().run(), <Code2 className="h-3.5 w-3.5" />)}
        {toolBtn(state.link, toggleLink, <LinkIcon className="h-3.5 w-3.5" />)}
        <span className="mx-1 h-4 w-px bg-border" />
        {toolBtn(false, () => editor.chain().focus().undo().run(), <Undo2 className="h-3.5 w-3.5" />)}
        {toolBtn(false, () => editor.chain().focus().redo().run(), <Redo2 className="h-3.5 w-3.5" />)}
      </div>
      <EditorContent
        editor={editor}
        className="mail-prose min-h-[10rem] flex-1 overflow-y-auto px-3 py-2"
      />
    </div>
  );
}
