"use client";

import { useLocale } from "next-intl";
import { useRouter } from "next/navigation";

const locales = [
  { code: "en", label: "EN" },
  { code: "zh", label: "中文" },
];

export function LocaleSwitcher() {
  const locale = useLocale();
  const router = useRouter();

  function switchTo(code: string) {
    document.cookie = `NEXT_LOCALE=${code}; path=/; max-age=31536000; samesite=lax`;
    router.refresh();
  }

  return (
    <div className="flex items-center gap-0.5">
      {locales.map((l) => (
        <button
          key={l.code}
          onClick={() => switchTo(l.code)}
          className={`rounded px-1.5 py-0.5 text-xs transition-colors ${
            locale === l.code
              ? "bg-zinc-200 font-medium text-zinc-900 dark:bg-zinc-700 dark:text-zinc-50"
              : "text-zinc-500 hover:bg-zinc-100 hover:text-zinc-900 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
          }`}
        >
          {l.label}
        </button>
      ))}
    </div>
  );
}
