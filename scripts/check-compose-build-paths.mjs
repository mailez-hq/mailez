#!/usr/bin/env node
// check-compose-build-paths: assert every build overlay's context +
// dockerfile pair actually resolves to an existing Dockerfile.
//
// Compose resolves `dockerfile` relative to `context` (not to the compose
// file), so `context: ../backend` + `dockerfile: backend/Dockerfile`
// silently points at backend/backend/Dockerfile. Bake uses context = repo
// root + dockerfile = backend/Dockerfile; overlays must match. This checker
// runs in CI (node, no docker daemon required) so a rename or a new overlay
// cannot reintroduce the drift again.
//
// External build contexts (the sibling mailezine checkout, e.g.
// ${MAILEZINE_CONTEXT:-../../mailezine}) resolve outside the repo root and
// are not present in every checkout; they are reported and skipped, not
// failed.
//
// Usage: node scripts/check-compose-build-paths.mjs   (from the repo root)
import {readFileSync, readdirSync, existsSync, statSync} from 'node:fs';
import {dirname, isAbsolute, join, relative, resolve} from 'node:path';
import {fileURLToPath} from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');

// Segment-boundary containment: a plain startsWith would treat the sibling
// engine checkout `mailezine` as inside this repo because the string
// "<root>/mailezine" happens to start with "<root>/mailez". Equal paths
// (context = the repo root itself) count as inside.
function isUnder(p, base) {
  const rel = relative(base, p);
  return rel === '' || (!rel.startsWith('..') && !isAbsolute(rel));
}

function walk(dir, out) {
  for (const entry of readdirSync(dir, {withFileTypes: true})) {
    if (entry.name === '.git' || entry.name === 'node_modules') continue;
    const full = join(dir, entry.name);
    if (entry.isDirectory()) walk(full, out);
    else if (/^docker-compose\.build.+\.ya?ml$/.test(entry.name)) out.push(full);
  }
  return out;
}

function expandContext(ctx) {
  const m = /^\$\{([A-Za-z_][A-Za-z0-9_]*):-(.*)\}$/.exec(ctx.trim());
  return m ? m[2] : ctx.trim();
}

function isDir(p) {
  try {
    return statSync(p).isDirectory();
  } catch {
    return false;
  }
}

const files = walk(join(root, 'deploy'), []);
let failed = false;
for (const file of files) {
  const content = readFileSync(file, 'utf8');
  const pairRe = /context:\s*([^\r\n]+)\r?\n\s*dockerfile:\s*([^\r\n]+)/g;
  let m;
  while ((m = pairRe.exec(content)) !== null) {
    const ctxValue = expandContext(m[1]);
    const ctxDir = resolve(dirname(file), ctxValue);
    const df = resolve(ctxDir, m[2].trim());
    const relFile = file.slice(root.length + 1);
    if (!isUnder(ctxDir, root)) {
      // External context: only resolvable when the sibling checkout exists
      // (CI for this repo does not always carry ../mailezine). Validate the
      // Dockerfile when the context is present, otherwise report and skip.
      if (isDir(ctxDir) && !existsSync(df)) {
        console.error(
          `FAIL: ${relFile}: external context present but dockerfile ${m[2].trim()} missing (resolved ${df})`,
        );
        failed = true;
      } else {
        console.log(`skip (external context): ${relFile}: ${ctxValue} + ${m[2].trim()}`);
      }
      continue;
    }
    if (!isDir(ctxDir)) {
      console.error(`FAIL: ${relFile}: build context does not exist: ${ctxValue} (resolved ${ctxDir})`);
      failed = true;
      continue;
    }
    if (!existsSync(df)) {
      console.error(
        `FAIL: ${relFile}: dockerfile ${m[2].trim()} not found under context ${ctxValue} (resolved ${df})`,
      );
      failed = true;
      continue;
    }
    console.log(`ok: ${relFile}: ${ctxValue} + ${m[2].trim()}`);
  }
}
if (failed) {
  console.error('compose build overlays contain unresolvable build paths');
  process.exit(1);
}
console.log('all compose build overlay paths resolve');
