import type { NextConfig } from "next";

// Local dev talks to the backend on 8080 (same port the backend listens on
// inside compose); container deployments override this with API_TARGET.
const API_TARGET = process.env.API_TARGET || "http://localhost:8080";

const nextConfig: NextConfig = {
  transpilePackages: ["@mailez/ui", "@mailez/types"],
  // The app is served behind nginx (gateway) which handles compression;
  // Next's own gzip would buffer proxied SSE frames (text/event-stream) and
  // the mailbox push would never reach the browser.
  compress: false,
  async rewrites() {
    return [
      { source: "/api/v1/:path*", destination: `${API_TARGET}/api/v1/:path*` },
    ];
  },
};

export default nextConfig;
