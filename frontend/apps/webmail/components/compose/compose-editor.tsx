"use client";

import { useEffect, useRef, useState } from "react";
import { EditorContent, useEditor, useEditorState } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import Placeholder from "@tiptap/extension-placeholder";
import { TextStyle } from "@tiptap/extension-text-style";
import { FontFamily } from "@tiptap/extension-font-family";
import { Color } from "@tiptap/extension-color";
import { Highlight } from "@tiptap/extension-highlight";
import { Image } from "@tiptap/extension-image";
import { Table } from "@tiptap/extension-table";
import { TableRow } from "@tiptap/extension-table-row";
import { TableCell } from "@tiptap/extension-table-cell";
import { TableHeader } from "@tiptap/extension-table-header";
import {
  Bold, Code2, Heading2, Heading3, ImagePlus, Italic, Link as LinkIcon,
  List, ListOrdered, Quote, Redo2, Smile, Strikethrough, Table as TableIcon,
  Trash2, Underline, Undo2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { quoteBlockText } from "@/components/mailbox/mail-utils";
import { CollapsibleBlockquote } from "./collapsible-blockquote";

// Font size rides on TextStyle so it survives serialization as inline style.
const FontSize = TextStyle.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      fontSize: {
        default: null,
        parseHTML: (el: HTMLElement) => el.style.fontSize || null,
        renderHTML: (attrs: Record<string, unknown>) =>
          attrs.fontSize ? {style: `font-size: ${attrs.fontSize}`} : {},
      },
    };
  },
  addCommands() {
    return {
      setFontSize:
        (size: string) =>
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        ({chain}: any) =>
          (chain().focus().setMark("textStyle", {fontSize: size}).run() as unknown) as boolean,
      unsetFontSize:
        () =>
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        ({chain}: any) =>
          (chain().focus().setMark("textStyle", {fontSize: null}).removeEmptyTextStyle().run() as unknown) as boolean,
    };
  },
});

const FONT_FAMILIES = [
  { label: "默认字体", value: "" },
  { label: "Arial", value: "Arial, Helvetica, sans-serif" },
  { label: "Georgia", value: "Georgia, 'Times New Roman', serif" },
  { label: "等宽", value: "Consolas, 'Courier New', monospace" },
  { label: "微软雅黑", value: "'Microsoft YaHei', 'PingFang SC', sans-serif" },
];

const FONT_SIZES = ["12px", "14px", "16px", "18px", "22px", "28px"];

const EMOJIS = [
  "😀", "😄", "😊", "🙂", "😉", "😍", "🤔", "😎", "😅", "😂",
  "👍", "👎", "👏", "🙏", "💪", "🤝", "🎉", "❤️", "⭐", "🔥",
  "✅", "❌", "❗", "❓", "📌", "📎", "📧", "📅", "☕", "🍀",
];

const MAX_INLINE_IMAGE_BYTES = 2 * 1024 * 1024;

// Rich-text compose editor. Emits HTML for the wire and plain text as a
// degraded body for recipients that only understand text/plain.
export function ComposeEditor({
  value,
  onChange,
  placeholder,
  autoFocus,
  spellcheck = true,
}: {
  value: string;
  onChange: (html: string, text: string) => void;
  placeholder?: string;
  autoFocus?: boolean;
  spellcheck?: boolean;
}) {
  // TipTap's deferred editor creation (immediatelyRender:false on Next.js)
  // emits one update right after mount, normalizing the document. That
  // normalization must not count as a user edit (it would dirty the store
  // body and trigger draft auto-save on a pristine reply), so the first
  // update of each editor lifecycle is ignored.
  const firstUpdate = useRef(true);
  const imageInputRef = useRef<HTMLInputElement>(null);
  const [emojiOpen, setEmojiOpen] = useState(false);
  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        heading: {levels: [2, 3]},
        link: {openOnClick: false},
        blockquote: false,
      }),
      CollapsibleBlockquote,
      Placeholder.configure({placeholder}),
      FontSize,
      FontFamily,
      Color,
      Highlight.configure({multicolor: true}),
      Image.configure({inline: false, allowBase64: true}),
      Table.configure({resizable: true}),
      TableRow,
      TableHeader,
      TableCell,
    ],
    content: value || "",
    editorProps: {
      attributes: {spellcheck: spellcheck ? "true" : "false"},
    },
    onUpdate: ({editor}) => {
      if (firstUpdate.current) {
        firstUpdate.current = false;
        return;
      }
      const html = editor.isEmpty ? "" : editor.getHTML();
      // Quotes must serialize as "> " lines (see serializeBlockquote); the
      // default block text separator would otherwise inject extra blank
      // lines into every reply and grow them across reply chains.
      onChange(
        html,
        editor.getText({
          textSerializers: {
            // textBetween (not textContent) so hard breaks inside the quote
            // stay single newlines instead of vanishing.
            blockquote: ({ node }) => quoteBlockText(node),
          },
        }) || "",
      );
    },
  });

  // keep the editor in sync when the value is replaced externally
  // (reply/forward quoting, AI drafts)
  useEffect(() => {
    if (!editor) return;
    const current = editor.isEmpty ? "" : editor.getHTML();
    if (value !== current) {
      // The store owns the content (quote/signature/AI drafts): syncing it
      // into the editor must not count as a user edit, otherwise mount
      // normalization would dirty the body and trigger draft auto-save.
      editor.commands.setContent(value || "", {emitUpdate: false});
    }
  }, [value, editor]);

  // Place the cursor on a fresh blank line above the quoted content on
  // reply/forward so the user can start typing right away. Exactly one empty
  // paragraph is inserted at the document start (no-op when the body already
  // starts empty); a short delay lets the dialog finish opening.
  useEffect(() => {
    if (!editor || !autoFocus) return;
    const t = setTimeout(() => {
      const first = editor.state.doc.firstChild;
      // A textblock with no content renders as a single blank line. Inserting
      // "<p><br></p>" would parse the <br> as a hard break and show TWO blank
      // lines, so insert an empty paragraph node instead.
      const startsEmpty = !!first && first.isTextblock && first.content.size === 0;
      if (!startsEmpty) {
        editor.commands.insertContentAt(0, {type: "paragraph"}, {updateSelection: true});
      }
      editor.commands.focus("start");
    }, 60);
    return () => clearTimeout(t);
  }, [editor, autoFocus]);

  const state = useEditorState({
    editor,
    selector: ({editor}) => {
      const is = (name: string, attrs?: Record<string, unknown>) =>
        editor?.isActive(name, attrs) ?? false;
      return {
        bold: is("bold"),
        italic: is("italic"),
        underline: is("underline"),
        strike: is("strike"),
        h2: is("heading", {level: 2}),
        h3: is("heading", {level: 3}),
        bullet: is("bulletList"),
        ordered: is("orderedList"),
        quote: is("blockquote"),
        code: is("codeBlock"),
        link: is("link"),
        table: is("table"),
        textStyle: editor?.getAttributes("textStyle") ?? {},
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
      editor.chain().focus().extendMarkRange("link").setLink({href}).run();
    }
  }

  function insertEmoji(e: string) {
    editor.chain().focus().insertContent(e).run();
    setEmojiOpen(false);
  }

  function onImagePicked(file: File | undefined) {
    if (!file) return;
    if (file.size > MAX_INLINE_IMAGE_BYTES) {
      window.alert("图片过大（上限 2MB），请压缩后重试");
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      const src = String(reader.result || "");
      if (src) editor.chain().focus().setImage({src}).run();
    };
    reader.readAsDataURL(file);
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

  // Open the hidden file picker. The ref is only dereferenced inside the
  // click handler; the lint rule conservatively flags the closure.
  const pickImage = () => imageInputRef.current?.click();

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-md border border-border">
      <input
        ref={imageInputRef}
        type="file"
        accept="image/*"
        className="hidden"
        onChange={(e) => {
          onImagePicked(e.target.files?.[0]);
          e.target.value = "";
        }}
      />
      <div className="flex shrink-0 flex-wrap items-center gap-0.5 border-b bg-muted/40 px-2 py-1">
        {toolBtn(state.bold, () => editor.chain().focus().toggleBold().run(), <Bold className="h-3.5 w-3.5" />)}
        {toolBtn(state.italic, () => editor.chain().focus().toggleItalic().run(), <Italic className="h-3.5 w-3.5" />)}
        {toolBtn(state.underline, () => editor.chain().focus().toggleUnderline().run(), <Underline className="h-3.5 w-3.5" />)}
        {toolBtn(state.strike, () => editor.chain().focus().toggleStrike().run(), <Strikethrough className="h-3.5 w-3.5" />)}
        <span className="mx-1 h-4 w-px bg-border" />
        {toolBtn(state.h2, () => editor.chain().focus().toggleHeading({level: 2}).run(), <Heading2 className="h-3.5 w-3.5" />)}
        {toolBtn(state.h3, () => editor.chain().focus().toggleHeading({level: 3}).run(), <Heading3 className="h-3.5 w-3.5" />)}
        <span className="mx-1 h-4 w-px bg-border" />
        {toolBtn(state.bullet, () => editor.chain().focus().toggleBulletList().run(), <List className="h-3.5 w-3.5" />)}
        {toolBtn(state.ordered, () => editor.chain().focus().toggleOrderedList().run(), <ListOrdered className="h-3.5 w-3.5" />)}
        {toolBtn(state.quote, () => editor.chain().focus().toggleBlockquote().run(), <Quote className="h-3.5 w-3.5" />)}
        {toolBtn(state.code, () => editor.chain().focus().toggleCodeBlock().run(), <Code2 className="h-3.5 w-3.5" />)}
        {toolBtn(state.link, toggleLink, <LinkIcon className="h-3.5 w-3.5" />)}
        <span className="mx-1 h-4 w-px bg-border" />
        <select
          title="字体"
          value={state.textStyle.fontFamily ?? ""}
          onChange={(e) => {
            const v = e.target.value;
            if (v) editor.chain().focus().setFontFamily(v).run();
            else editor.chain().focus().unsetFontFamily().run();
          }}
          className="h-7 rounded border border-border bg-background px-1 text-xs"
        >
          {FONT_FAMILIES.map((f) => (
            <option key={f.value || "default"} value={f.value}>{f.label}</option>
          ))}
        </select>
        <select
          title="字号"
          value={state.textStyle.fontSize ?? ""}
          onChange={(e) => {
            const v = e.target.value;
            if (v) editor.chain().focus().setFontSize(v).run();
            else editor.chain().focus().unsetFontSize().run();
          }}
          className="h-7 rounded border border-border bg-background px-1 text-xs"
        >
          <option value="">字号</option>
          {FONT_SIZES.map((s) => (
            <option key={s} value={s}>{s}</option>
          ))}
        </select>
        <label
          title="文字颜色"
          className="relative flex h-7 w-7 cursor-pointer items-center justify-center rounded text-xs hover:bg-muted"
        >
          <span className="text-sm font-semibold">A</span>
          <input
            type="color"
            value={(state.textStyle.color as string) || "#000000"}
            onChange={(e) => editor.chain().focus().setColor(e.target.value).run()}
            className="absolute h-0 w-0 opacity-0"
          />
        </label>
        <label
          title="高亮颜色"
          className="relative flex h-7 w-7 cursor-pointer items-center justify-center rounded text-xs hover:bg-muted"
        >
          <span className="rounded-sm bg-yellow-200 px-1 text-sm font-semibold">A</span>
          <input
            type="color"
            value={(state.textStyle.highlight as string) || "#fef08a"}
            onChange={(e) => editor.chain().focus().toggleHighlight({color: e.target.value}).run()}
            className="absolute h-0 w-0 opacity-0"
          />
        </label>
        <span className="mx-1 h-4 w-px bg-border" />
        <div className="relative">
          {toolBtn(emojiOpen, () => setEmojiOpen((v) => !v), <Smile className="h-3.5 w-3.5" />, "Emoji")}
          {emojiOpen && (
            <div className="absolute top-full left-0 z-30 mt-1 grid w-56 grid-cols-8 gap-0.5 rounded-lg border border-border bg-popover p-1.5 shadow-lg">
              {EMOJIS.map((e) => (
                <button
                  key={e}
                  type="button"
                  onClick={() => insertEmoji(e)}
                  className="rounded p-1 text-base hover:bg-muted"
                >
                  {e}
                </button>
              ))}
            </div>
          )}
        </div>
        {/* eslint-disable-next-line react-hooks/refs */}
        {toolBtn(false, pickImage, <ImagePlus className="h-3.5 w-3.5" />, "插入图片")}
        {!state.table
          ? toolBtn(false, () => editor.chain().focus().insertTable({rows: 3, cols: 3, withHeaderRow: true}).run(), <TableIcon className="h-3.5 w-3.5" />, "插入表格")
          : toolBtn(false, () => editor.chain().focus().deleteTable().run(), <Trash2 className="h-3.5 w-3.5" />, "删除表格")}
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
