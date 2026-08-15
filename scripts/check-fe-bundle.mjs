import { readdirSync, readFileSync, statSync } from "node:fs";
import { fileURLToPath } from "node:url";

const dist = fileURLToPath(new URL("../internal/web/dist/", import.meta.url));
const assets = new URL("assets/", `file://${dist}`);
const maxChunkBytes = 500 * 1024;
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
const initial = [...new Set(initialNames)].map((name) => ({ name, bytes: statSync(new URL(name, assets)).size }));
const initialBytes = initial.reduce((total, file) => total + file.bytes, 0);

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
    `initial ${kib(initialBytes)} across ${initial.length} files`,
);
