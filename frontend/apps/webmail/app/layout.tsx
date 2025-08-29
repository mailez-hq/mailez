import type { Metadata } from "next";
import { cookies } from "next/headers";
import { Geist, Geist_Mono } from "next/font/google";
import { Providers } from "@/components/providers";
import { PreferencesProvider } from "@/components/preferences-provider";
import { ServiceWorkerRegister } from "@/components/service-worker-register";
import { parseThemeCookie } from "@/lib/preferences";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "Mailez Webmail",
  description: "Mailez webmail — mail easy",
  manifest: "/manifest.webmanifest",
  themeColor: "#2E6E8E",
  appleWebApp: {
    capable: true,
    title: "Mailez",
    statusBarStyle: "default",
  },
};

// Supported locales; the language lives in the NEXT_LOCALE cookie only and the
// URL never carries a locale segment.
const locales = ["en", "zh"] as const;
type Locale = (typeof locales)[number];

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const store = await cookies();
  const cookieLocale = store.get("NEXT_LOCALE")?.value as Locale | undefined;
  const locale: Locale = cookieLocale && locales.includes(cookieLocale) ? cookieLocale : "en";
  const messages = (await import(`../messages/${locale}.json`)).default;
  // Theme lives in a cookie (mirrored from preferences on every apply) so
  // the server paints dark/density/accent straight onto <html> — no
  // bootstrap script and no theme flash. React 19 refuses to execute
  // scripts rendered through client components, which rules the old
  // inline-bootstrap approach out entirely.
  const theme = parseThemeCookie(store.get("mailez.theme")?.value);

  return (
    <html
      lang={locale}
      suppressHydrationWarning
      className={`${geistSans.variable} ${geistMono.variable}${theme?.dark ? " dark" : ""}`}
      {...(theme ? { "data-density": theme.density, "data-accent": theme.accent } : {})}
    >
      <body className="min-h-screen bg-background font-sans text-foreground">
        <PreferencesProvider>
          <Providers locale={locale} messages={messages}>
            {children}
          </Providers>
        </PreferencesProvider>
        <ServiceWorkerRegister />
      </body>
    </html>
  );
}
