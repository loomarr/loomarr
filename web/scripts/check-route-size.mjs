// The route-size ratchet (#4706). A route file keeps its params, loader and component wiring; page
// bodies live in feature folders. A route file over LIMIT lines must be in apps/web/route-size.json
// at no fewer lines than it has. Counts that fall pass; `--write` only lowers or drops entries.
// With no ledger, `--write` records the current state.
import { existsSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

const LIMIT = 60;
const appRoot = fileURLToPath(new URL("../apps/web/", import.meta.url));
const ledgerPath = join(appRoot, "route-size.json");
const NOTE =
  "Route files over 60 lines, maintained by web/scripts/check-route-size.mjs. Counts may only fall: `pnpm route-size:update` shrinks them. Never raise one to pass the check.";

// The same files check-route-loaders.mjs treats as routes.
const isRouteFile = (name) =>
  /\.tsx?$/.test(name) && !name.startsWith("-") && !/\.(?:test|stories)\./.test(name);

const routeFiles = (directory) =>
  readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return routeFiles(path);
    return isRouteFile(entry.name) ? [path] : [];
  });

const lineCount = (source) => source.replace(/\n$/, "").split("\n").length;

const oversized = (sizes) => Object.fromEntries(Object.entries(sizes).filter(([, n]) => n > LIMIT));

const violations = (sizes, ledger) =>
  Object.entries(oversized(sizes))
    .filter(([file, n]) => n > (ledger[file] ?? 0))
    .map(([file, n]) => `${file}: ${n} lines (allowed ${ledger[file] ?? LIMIT})`);

const shrink = (sizes, ledger) =>
  Object.fromEntries(
    Object.entries(ledger)
      .filter(([file]) => (sizes[file] ?? 0) > LIMIT)
      .map(([file, allowed]) => [file, Math.min(allowed, sizes[file])]),
  );

const main = () => {
  const sizes = Object.fromEntries(
    routeFiles(join(appRoot, "src", "routes"))
      .map((path) => [relative(appRoot, path).split(sep).join("/"), lineCount(readFileSync(path, "utf8"))])
      .sort(([a], [b]) => a.localeCompare(b)),
  );
  const ledger = existsSync(ledgerPath) ? JSON.parse(readFileSync(ledgerPath, "utf8")).files : undefined;

  if (process.argv.includes("--write")) {
    const files = ledger ? shrink(sizes, ledger) : oversized(sizes);
    writeFileSync(ledgerPath, `${JSON.stringify({ note: NOTE, files }, null, 2)}\n`);
    return;
  }
  const found = violations(sizes, ledger ?? {});
  if (found.length > 0) {
    console.error(
      `Route files grew past route-size.json. Move the page into a feature folder:\n  ${found.join("\n  ")}`,
    );
    process.exit(1);
  }
  console.log("route-size: within the ledger");
};

if (process.argv[1] === fileURLToPath(import.meta.url)) main();

export { LIMIT, lineCount, shrink, violations };
