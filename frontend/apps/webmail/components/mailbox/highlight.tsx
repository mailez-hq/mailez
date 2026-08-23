"use client";

import { useMemo } from "react";

// highlightTerms extracts plain search words from a FastMail-style query,
// dropping field operators (from:, has:attachment, before:, ...) and quoted
// values so only free text is highlighted in the results.
export function highlightTerms(query: string): string[] {
  const terms: string[] = [];
  for (const m of query.matchAll(/([a-z]+):"(?:[^"]*)"|([a-z]+):\S+|\S+/gi)) {
    const tok = m[0];
    if (/^[a-z]+:/.test(tok)) continue;
    const word = tok.replace(/^"(.*)"$/, "$1").toLowerCase();
    if (word.length >= 2) terms.push(word);
  }
  return terms;
}

function escapeRegExp(s: string) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// Highlight renders text with matching terms wrapped in <mark>. Terms are
// matched case-insensitively; when none match the original text is returned.
export function Highlight({
  text,
  terms,
  className,
}: {
  text: string;
  terms?: string[];
  className?: string;
}) {
  const parts = useMemo(() => {
    if (!text || !terms || terms.length === 0) return [text];
    const re = new RegExp(`(${terms.map(escapeRegExp).join("|")})`, "gi");
    return text.split(re);
  }, [text, terms]);

  if (!terms || terms.length === 0) return <>{text}</>;
  return (
    <span className={className}>
      {parts.map((p, i) =>
        p && terms.some((term) => term !== "" && p.toLowerCase() === term.toLowerCase()) ? (
          <mark
            key={i}
            className="rounded-[2px] bg-[#C9A227]/30 px-px text-inherit"
          >
            {p}
          </mark>
        ) : (
          <span key={i}>{p}</span>
        ),
      )}
    </span>
  );
}
