"use client";

import { useRef, useState } from "react";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";

export type RecipientSuggestion = { name?: string; email: string };

// RecipientInput is a chip-style address input: type an address, press Enter
// or comma to pin it as a chip, click the chip's × to remove it, and pick from
// contact suggestions while typing. FastMail-style recipient editing.
export function RecipientInput({
  value,
  onChange,
  placeholder,
  autoFocus,
  suggestions,
  inputRef,
}: {
  value: string[];
  onChange: (v: string[]) => void;
  placeholder?: string;
  autoFocus?: boolean;
  suggestions?: RecipientSuggestion[];
  inputRef?: React.RefObject<HTMLInputElement | null>;
}) {
  const [text, setText] = useState("");
  const [open, setOpen] = useState(false);
  const localRef = useRef<HTMLInputElement>(null);
  const focusRef = inputRef ?? localRef;

  const add = (raw: string) => {
    const addr = raw.trim();
    if (!addr) return;
    if (!value.includes(addr)) onChange([...value, addr]);
    setText("");
  };

  const remove = (i: number) => onChange(value.filter((_, j) => j !== i));

  const filtered = (suggestions || []).filter(
    (s) =>
      (s.email.toLowerCase().includes(text.toLowerCase()) ||
        (s.name || "").toLowerCase().includes(text.toLowerCase())) &&
      !value.includes(s.email),
  );

  return (
    <div
      onClick={() => focusRef.current?.focus()}
      className="relative flex min-h-9 w-full flex-wrap items-center gap-1 rounded-md border border-input bg-background px-2 py-1 text-sm focus-within:ring-2 focus-within:ring-ring"
    >
      {value.map((addr, i) => (
        <span
          key={`${addr}-${i}`}
          className="inline-flex max-w-full items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-xs text-foreground"
        >
          <span className="truncate">{addr}</span>
          <button
            type="button"
            onClick={() => remove(i)}
            aria-label="remove"
            className="shrink-0 text-muted-foreground hover:text-foreground"
          >
            <X className="size-3" />
          </button>
        </span>
      ))}
      <input
        ref={focusRef}
        value={text}
        autoFocus={autoFocus}
        onChange={(e) => {
          setText(e.target.value);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => {
          setTimeout(() => setOpen(false), 150);
          add(text);
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === ",") {
            e.preventDefault();
            add(text);
          } else if (e.key === "Backspace" && text === "" && value.length > 0) {
            remove(value.length - 1);
          } else if (e.key === "Escape") {
            setOpen(false);
          }
        }}
        placeholder={value.length === 0 ? placeholder : ""}
        className="min-w-24 flex-1 bg-transparent outline-none placeholder:text-muted-foreground"
      />
      {open && filtered.length > 0 && (
        <div
          className={cn(
            "absolute left-0 right-0 top-full z-20 mt-1 rounded-lg border border-border bg-popover p-1 shadow-lg",
          )}
        >
          {filtered.map((s) => (
            <button
              key={s.email}
              type="button"
              onMouseDown={(e) => {
                e.preventDefault();
                add(s.email);
              }}
              className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors hover:bg-accent"
            >
              <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-muted text-[10px] font-semibold text-muted-foreground">
                {(s.name || s.email).charAt(0).toUpperCase()}
              </span>
              <span className="min-w-0 truncate">
                <span className="block truncate font-medium">{s.name || s.email}</span>
                {s.name && (
                  <span className="block truncate text-xs text-muted-foreground">{s.email}</span>
                )}
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
