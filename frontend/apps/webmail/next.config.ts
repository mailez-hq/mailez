import { cpSync, existsSync, rmSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import type { NextConfig } from "next";

// Materialize the edition modules before anything compiles: code imports
// enterprise-gated modules only through "@/edition/<name>", and edition/ is
// regenerated from ee/ (default, full-featured) or ce-stubs/
// (MAILEZ_EDITION=ce) by frontend/scripts/set-edition.mjs. edition/ is a
// gitignored build artifact; the public CE export strips ee/ entirely.
const appRoot = dirname(resolve(fileURLToPath(import.meta.url)));
const editionSrc = (process.env.MAILEZ_EDITION ?? "ee").toLowerCase() === "ce" ? "ce-stubs" : "ee";
const editionSrcDir = join(appRoot, editionSrc);
if (!existsSync(editionSrcDir)) {
  throw new Error(`next.config: missing ${editionSrc}/ directory under ${appRoot}`);
}
rmSync(join(appRoot, "edition"), { recursive: true, force: true });
cpSync(editionSrcDir, join(appRoot, "edition"), { recursive: true });

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
