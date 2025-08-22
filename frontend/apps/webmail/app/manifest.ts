import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "mailez Webmail",
    short_name: "mailez",
    description: "mailez webmail — mail easy",
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
