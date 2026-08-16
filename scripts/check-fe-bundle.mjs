import { readdirSync, readFileSync, statSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { gzipSync } from "node:zlib";

const dist = fileURLToPath(new URL("../internal/web/dist/", import.meta.url));
const assets = new URL("assets/", `file://${dist}`);
// ⚠ RAW bytes, and this budget governs LAZY chunks — the one that guards load time is
// maxInitialBytes below, which only counts what index.html actually pulls in up front.
//
// Raised from 500 KiB when hls.js 1.6.17 -> 1.7.0 took the video chunk to 559.5 KiB. That
// chunk is not in index.html (it loads when a player opens) and is 172 KiB over the wire
// gzipped, so nothing a user waits for got meaningfully bigger; the dependency just grew
// past a raw-byte line drawn while it happened to sit under one. 640 KiB leaves the vendor
// player room to move without going back to a number that a routine bump re-trips.
const maxChunkBytes = 640 * 1024;
const maxInitialBytes = 1024 * 1024;

const jsFiles = readdirSync(assets)
  .filter((name) => name.endsWith(".js"))
  .map((name) => ({ name, bytes: statSync(new URL(name, assets)).size }))
  .sort((a, b) => b.bytes - a.bytes);

const oversized = jsFiles.filter(({ bytes }) => bytes > maxChunkBytes);
const html = readFileSync(new URL("index.html", `file://${dist}`), "utf8");
const initialNames = [
  ...html.matchAll(/<script[^>]+src="\/assets\/([^"]+\.js)"/g),
  ...html.matchAll(/<link[^>]+rel="modulepreload"[^>]+href="\/assets\/([^"]+\.js)"/g),
].map((match) => match[1]);
const initial = [...new Set(initialNames)].map((name) => {
  const url = new URL(name, assets);
  return { name, bytes: statSync(url).size, gzipBytes: gzipSync(readFileSync(url)).length };
});
const initialBytes = initial.reduce((total, file) => total + file.bytes, 0);
const initialGzipBytes = initial.reduce((total, file) => total + file.gzipBytes, 0);

const kib = (bytes) => `${(bytes / 1024).toFixed(1)} KiB`;
const failures = [];
if (oversized.length > 0) {
  failures.push(`oversized chunks: ${oversized.map(({ name, bytes }) => `${name} (${kib(bytes)})`).join(", ")}`);
}
if (initialBytes > maxInitialBytes) {
  failures.push(`initial JavaScript: ${kib(initialBytes)} across ${initial.length} files`);
}

if (failures.length > 0) {
  throw new Error(
    `Frontend bundle budget exceeded\n${failures.map((failure) => `  - ${failure}`).join("\n")}\n` +
      `Budgets: ${kib(maxChunkBytes)} per chunk, ${kib(maxInitialBytes)} initial JavaScript.`,
  );
}

console.log(
  `bundle-size: largest ${jsFiles[0]?.name ?? "none"} (${kib(jsFiles[0]?.bytes ?? 0)}); ` +
    `initial ${kib(initialBytes)} raw / ${kib(initialGzipBytes)} gzip across ${initial.length} files`,
);
