import type { Metadata } from "next";
import { cookies } from "next/headers";
import { Geist, Geist_Mono } from "next/font/google";
import Script from "next/script";
import { Providers } from "@/components/providers";
import { PreferencesProvider } from "@/components/preferences-provider";
import { ServiceWorkerRegister } from "@/components/service-worker-register";
import { themeBootstrapScript } from "@/lib/preferences";
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

  return (
    <html lang={locale} suppressHydrationWarning className={`${geistSans.variable} ${geistMono.variable}`}>
      <body className="min-h-screen bg-background font-sans text-foreground">
        {/* Theme bootstrap: beforeInteractive injects the snippet into the
            initial HTML on the server so the dark/density classes land
            before first paint (no flash). A raw <script> tag is not an
            option in client-rendered components — React never executes it
            there. */}
        <Script id="theme-bootstrap" strategy="beforeInteractive" dangerouslySetInnerHTML={{ __html: themeBootstrapScript }} />
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
