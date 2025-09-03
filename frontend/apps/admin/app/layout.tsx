import type { Metadata } from "next";
import { cookies } from "next/headers";
import { Geist, Geist_Mono } from "next/font/google";
import { Providers } from "@/components/providers";
import { fetchPublicBrand } from "@/lib/branding";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

// White-label: the tab title follows the admin-console branding. The layout
// renders dynamically (cookies below), so this fetch runs per request; a
// down backend degrades to the built-in Mailez brand.
export async function generateMetadata(): Promise<Metadata> {
  const brand = await fetchPublicBrand();
  return {
    title: brand ? `${brand.title} · Admin` : "Mailez · Admin",
    description: brand
      ? `${brand.title} mail server admin console`
      : "Mailez mail server admin console",
  };
}

// Supported locales; the language lives in the NEXT_LOCALE cookie only and the
// URL never carries a locale segment.
const locales = ["en", "zh"] as const;
type Locale = (typeof locales)[number];

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const store = await cookies();
  const cookieLocale = store.get("NEXT_LOCALE")?.value as Locale | undefined;
  const locale: Locale = cookieLocale && locales.includes(cookieLocale) ? cookieLocale : "en";
  const messages = (await import(`../messages/${locale}.json`)).default;

  return (
    <html lang={locale} className={`${geistSans.variable} ${geistMono.variable}`}>
      <body className="min-h-screen font-sans antialiased">
        <Providers locale={locale} messages={messages}>
          {children}
        </Providers>
      </body>
    </html>
  );
}
