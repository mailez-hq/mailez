import type { MetadataRoute } from "next";
import { fetchPublicBrand } from "@/lib/branding";

// The PWA name follows the admin-console branding; without it the installed
// app keeps the built-in Mailez name. Route handlers cache by default, so
// opt out explicitly — the brand must resolve per request.
export const dynamic = "force-dynamic";

export default async function manifest(): Promise<MetadataRoute.Manifest> {
  const brand = await fetchPublicBrand();
  return {
    name: brand ? `${brand.title} Webmail` : "Mailez Webmail",
    short_name: brand?.title ?? "Mailez",
    description: brand
      ? `${brand.title} webmail — mail easy`
      : "Mailez webmail — mail easy",
    start_url: "/",
    display: "standalone",
    background_color: "#f8fafc",
    theme_color: "#2E6E8E",
    icons: [
      {
        src: "/mailez-icon.svg",
        sizes: "any",
        type: "image/svg+xml",
      },
    ],
  };
}
