import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "Mailez Webmail",
    short_name: "Mailez",
    description: "Mailez webmail — mail easy",
    start_url: "/",
    display: "standalone",
    background_color: "#fafaf7",
    theme_color: "#2f8e6c",
    icons: [
      {
        src: "/mailez-icon.svg",
        sizes: "any",
        type: "image/svg+xml",
      },
    ],
  };
}
