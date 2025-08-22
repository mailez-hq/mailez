"use client";

import { NextIntlClientProvider } from "next-intl";
import type { AbstractIntlMessages } from "use-intl";

export function Providers({ locale, messages, children }: {
  locale: string;
  messages: AbstractIntlMessages;
  children: React.ReactNode;
}) {
  return (
    <NextIntlClientProvider locale={locale} messages={messages}>
      {children}
    </NextIntlClientProvider>
  );
}
