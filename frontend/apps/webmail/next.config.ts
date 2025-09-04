import { cpSync, existsSync, rmSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import type { NextConfig } from "next";

// Materialize the swappable module set before anything compiles: code
// imports swappable modules only through "@/modules/<name>", and modules/
// is regenerated from the directory named by MAILEZ_MODULES (default:
// "default") by frontend/scripts/set-modules.mjs. modules/ is a gitignored
// build artifact.
const appRoot = dirname(resolve(fileURLToPath(import.meta.url)));
const moduleSet = process.env.MAILEZ_MODULES || "default";
const moduleSrcDir = join(appRoot, moduleSet);
if (!existsSync(moduleSrcDir)) {
  throw new Error(`next.config: missing ${moduleSet}/ directory under ${appRoot}`);
}
rmSync(join(appRoot, "modules"), { recursive: true, force: true });
cpSync(moduleSrcDir, join(appRoot, "modules"), { recursive: true });

// Local dev talks to the backend on 8080 (same port the backend listens on
// inside compose); container deployments override this with API_TARGET.
const API_TARGET = process.env.API_TARGET || "http://localhost:8080";

const nextConfig: NextConfig = {
  transpilePackages: ["@mailez/ui", "@mailez/types"],
  // Browsers may resolve localhost to IPv6 ([::1]:3001); Next blocks
  // cross-origin dev resources unless the alternate origin is allowed.
  allowedDevOrigins: ["localhost", "[::1]"],
  // Client-side marker for module-aware helpers: "true" when an extended
  // module set is baked in, empty for the default set — helpers short-
  // circuit instead of firing requests that can only 404.
  env: {
    NEXT_PUBLIC_MAILEZ_MODULES_ACTIVE: moduleSet === "default" ? "" : "true",
  },
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
