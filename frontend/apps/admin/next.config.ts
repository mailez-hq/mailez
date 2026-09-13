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

// The admin console is served under /admin: the gateway routes
// location /admin to this app (deploy/overrides/nginx/admin.conf), so one
// webmail domain serves both apps and the direct host port can stay on
// loopback. Next prefixes pages, assets and rewrites automatically; raw
// absolute strings in code must use BASE_PATH from lib/api. Overridable
// (empty string or another prefix) for deployments serving at a domain root.
const BASE_PATH = process.env.NEXT_PUBLIC_BASE_PATH ?? "/admin";

const nextConfig: NextConfig = {
  basePath: BASE_PATH,
  transpilePackages: ["@mailez/ui", "@mailez/types"],
  // Client-side marker for module-aware helpers: "true" when an extended
  // module set is baked in, empty for the default set — helpers short-
  // circuit instead of firing requests that can only 404.
  env: {
    NEXT_PUBLIC_MAILEZ_MODULES_ACTIVE: moduleSet === "default" ? "" : "true",
    NEXT_PUBLIC_BASE_PATH: BASE_PATH,
  },
  async rewrites() {
    return [
      { source: "/api/v1/:path*", destination: `${API_TARGET}/api/v1/:path*` },
      // Client autoconfiguration + MTA-STS served by the backend (the nginx
      // gateway rewrites the same URLs to /stack/autoconfig/* in mail
      // deployments; these cover gateway-less web deployments).
      { source: "/.well-known/autoconfig/:path*", destination: `${API_TARGET}/.well-known/autoconfig/:path*` },
      { source: "/mail/config-v1.1.xml", destination: `${API_TARGET}/mail/config-v1.1.xml` },
      { source: "/.well-known/mta-sts.txt", destination: `${API_TARGET}/.well-known/mta-sts.txt` },
      { source: "/autodiscover/:path*", destination: `${API_TARGET}/autodiscover/:path*` },
    ];
  },
};

export default nextConfig;
