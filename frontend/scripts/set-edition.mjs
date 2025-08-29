#!/usr/bin/env node
// set-edition: materialize the edition modules for a Next.js app before
// build/typecheck. The app imports edition-gated modules only through
// "@/edition/<name>"; this script copies either ee/ (enterprise) or
// ce-stubs/ (community) into edition/ according to MAILEZ_EDITION
// (default: ee — internal development builds stay full-featured; public
// CE CI must set MAILEZ_EDITION=ce explicitly).
//
//   node frontend/scripts/set-edition.mjs frontend/apps/webmail
//
// edition/ is a build artifact: it is gitignored and regenerated per
// build, so the public CE export can simply strip the ee/ directory.
import { cpSync, existsSync, rmSync } from "node:fs";
import { resolve } from "node:path";

const appDir = resolve(process.argv[2] ?? ".");
const edition = (process.env.MAILEZ_EDITION ?? "ee").toLowerCase() === "ce" ? "ce-stubs" : "ee";
const src = resolve(appDir, edition);
if (!existsSync(src)) {
  console.error(`set-edition: ${src} not found (expected ee/ or ce-stubs/)`);
  process.exit(1);
}
const dst = resolve(appDir, "edition");
rmSync(dst, { recursive: true, force: true });
cpSync(src, dst, { recursive: true });
console.log(`set-edition: ${edition} -> ${dst}`);
