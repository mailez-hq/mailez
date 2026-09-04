#!/usr/bin/env node
// set-modules: materialize the swappable module set for a Next.js app
// before build/typecheck. The app imports swappable modules only through
// "@/modules/<name>"; this script copies the directory named by
// MAILEZ_MODULES (default: "default" — the in-repo baseline set) into
// modules/.
//
//   node frontend/scripts/set-modules.mjs frontend/apps/webmail
//
// modules/ is a build artifact: it is gitignored and regenerated per build.
import { cpSync, existsSync, rmSync } from "node:fs";
import { resolve } from "node:path";

const appDir = resolve(process.argv[2] ?? ".");
const moduleSet = process.env.MAILEZ_MODULES || "default";
const src = resolve(appDir, moduleSet);
if (!existsSync(src)) {
  console.error(`set-modules: ${src} not found (expected module set directory)`);
  process.exit(1);
}
const dst = resolve(appDir, "modules");
rmSync(dst, { recursive: true, force: true });
cpSync(src, dst, { recursive: true });
console.log(`set-modules: ${moduleSet} -> ${dst}`);
