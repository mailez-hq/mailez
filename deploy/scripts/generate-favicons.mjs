#!/usr/bin/env node
// Generates the gateway favicon set from branding/mailez-icon.svg into
// deploy/images/gateway/static. Run from the repo root:
//
//   node deploy/scripts/generate-favicons.mjs
//
// Requires the frontend workspace's sharp (pnpm install already done).
import { createRequire } from "node:module";
import { readFile, writeFile } from "node:fs/promises";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const sharp = require("../../frontend/node_modules/sharp");

const root = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const src = resolve(root, "branding/mailez-icon.svg");
const outDir = resolve(root, "deploy/images/gateway/static");
const svg = await readFile(src);

const pngTargets = {
  "favicon-16x16.png": 16,
  "favicon-32x32.png": 32,
  "mstile-150x150.png": 150,
  "apple-touch-icon.png": 180,
  "android-chrome-192x192.png": 192,
  "android-chrome-512x512.png": 512,
};

for (const [name, size] of Object.entries(pngTargets)) {
  const png = await sharp(svg, { density: 384 })
    .resize(size, size)
    .png()
    .toBuffer();
  await writeFile(resolve(outDir, name), png);
}

// favicon.ico: an ICO container holding PNG entries for 16/32/48.
const icoSizes = [16, 32, 48];
const entries = [];
for (const size of icoSizes) {
  entries.push({
    png: await sharp(svg, { density: 384 }).resize(size, size).png().toBuffer(),
  });
}
const header = Buffer.alloc(6);
header.writeUInt16LE(0, 0); // reserved
header.writeUInt16LE(1, 2); // type: icon
header.writeUInt16LE(entries.length, 4);
let offset = 6 + 16 * entries.length;
const dirs = [];
for (let i = 0; i < entries.length; i++) {
  const { png } = entries[i];
  const dir = Buffer.alloc(16);
  dir.writeUInt8(icoSizes[i], 0);
  dir.writeUInt8(icoSizes[i], 1);
  dir.writeUInt8(0, 2);
  dir.writeUInt8(0, 3);
  dir.writeUInt16LE(1, 4); // planes
  dir.writeUInt16LE(32, 6); // bit count
  dir.writeUInt32LE(png.length, 8);
  dir.writeUInt32LE(offset, 12);
  offset += png.length;
  dirs.push(dir);
}
await writeFile(
  resolve(outDir, "favicon.ico"),
  Buffer.concat([header, ...dirs, ...entries.map((e) => e.png)]),
);

console.log(`favicons written to ${outDir}`);
