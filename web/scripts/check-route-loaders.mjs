// The route-loader ratchet (#3399 item 9.2). Every route declares the data its page needs to paint
// with a `loader` that ensures the generated query options, so `defaultPreload: "intent"` fetches
// data together with code and a page arrives with its content instead of a spinner.
//
// A route file under apps/web/src/routes is SETTLED when its createFileRoute options carry a
// `loader`, or when it fetches nothing and says so with a `// no-loader: <reason>` comment (a
// redirect, or a layout that only renders its outlet). Files starting with `-` are not routes, and
// neither are tests.
//
// The committed ledger (apps/web/route-loaders.json) lists the routes that are not settled yet. It
// is a ratchet:
//   - the check fails when an unsettled route is not in the ledger (new debt);
//   - the check fails when a ledger entry is settled or gone, so the list is deleted from as routes
//     gain loaders and its length only falls;
//   - `--write` removes those entries. It never adds one, so regenerating cannot accept new debt;
//     that takes a hand edit to the JSON, visible in review. With no ledger yet, `--write` records
//     the current state.
import { existsSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { parse } from "@babel/parser";

const appRoot = fileURLToPath(new URL("../apps/web/", import.meta.url));
const routesRoot = join(appRoot, "src", "routes");
const ledgerPath = join(appRoot, "route-loaders.json");

const LEDGER_NOTE =
  "Routes with no loader yet (#3399 item 9.2), maintained by web/scripts/check-route-loaders.mjs. Entries may only be removed: give the route a loader (or a `// no-loader: <reason>` comment) and run `pnpm route-loaders:update`. Never add one to pass the check.";

const NO_LOADER_COMMENT = /\/\/\s*no-loader:\s*\S/;

const isRouteFile = (name) =>
  /\.tsx?$/.test(name) && !name.startsWith("-") && !/\.(?:test|stories)\./.test(name);

const routeFiles = (directory) =>
  readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return routeFiles(path);
    return isRouteFile(entry.name) ? [path] : [];
  });

const keyName = (key) =>
  key?.type === "Identifier" ? key.name : key?.type === "StringLiteral" ? key.value : undefined;

// The options object of `createFileRoute("/path")({ ... })`, or undefined when the file declares no
// such route.
const routeOptions = (node) => {
  if (!node || typeof node !== "object") return undefined;
  if (Array.isArray(node)) {
    for (const child of node) {
      const found = routeOptions(child);
      if (found) return found;
    }
    return undefined;
  }
  if (
    node.type === "CallExpression" &&
    node.callee.type === "CallExpression" &&
    node.callee.callee.type === "Identifier" &&
    node.callee.callee.name === "createFileRoute" &&
    node.arguments[0]?.type === "ObjectExpression"
  ) {
    return node.arguments[0];
  }
  for (const [key, child] of Object.entries(node)) {
    if (key === "loc" || key === "extra" || typeof child !== "object") continue;
    const found = routeOptions(child);
    if (found) return found;
  }
  return undefined;
};

// routeStatus reads one route file: "settled", "unsettled", or "none" when it declares no route.
const routeStatus = (source) => {
  const program = parse(source, { sourceType: "module", plugins: ["typescript", "jsx"] }).program;
  const options = routeOptions(program.body);
  if (!options) return "none";
  const hasLoader = options.properties.some(
    (property) =>
      (property.type === "ObjectProperty" || property.type === "ObjectMethod") &&
      keyName(property.key) === "loader",
  );
  return hasLoader || NO_LOADER_COMMENT.test(source) ? "settled" : "unsettled";
};

// findUnsettled returns the sorted "src/routes/…" paths of every route with no loader.
const findUnsettled = (root = routesRoot) => {
  const unsettled = [];
  for (const file of routeFiles(root).sort()) {
    if (routeStatus(readFileSync(file, "utf8")) === "unsettled") {
      unsettled.push(relative(appRoot, file).split(sep).join("/"));
    }
  }
  return unsettled;
};

// compareToLedger lists what the ledger does not allow: unsettled routes it does not list, and
// entries that are settled (or gone) and so must come off.
const compareToLedger = (unsettled, ledger) => {
  const violations = [];
  for (const file of unsettled) {
    if (!ledger.includes(file))
      violations.push(`${file}: no loader; add one that ensures the page's query options`);
  }
  for (const file of ledger) {
    if (!unsettled.includes(file)) {
      violations.push(`${file}: has a loader now (or is gone); run pnpm route-loaders:update to drop it`);
    }
  }
  return violations;
};

const readLedger = () => JSON.parse(readFileSync(ledgerPath, "utf8")).files;

const writeLedger = (files) =>
  writeFileSync(ledgerPath, `${JSON.stringify({ note: LEDGER_NOTE, files }, null, 2)}\n`);

const isMain = process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1];

if (isMain) {
  const unsettled = findUnsettled();
  if (process.argv.includes("--write")) {
    // Only ever removes entries; a missing ledger is recorded as it stands.
    const ledger = existsSync(ledgerPath) ? readLedger() : unsettled;
    writeLedger(ledger.filter((file) => unsettled.includes(file)));
    console.log(`route-loaders: ledger holds ${unsettled.length} route(s) with no loader`);
  } else {
    const violations = compareToLedger(unsettled, existsSync(ledgerPath) ? readLedger() : []);
    if (violations.length > 0) {
      for (const violation of violations) console.error(`route-loaders: ${violation}`);
      process.exitCode = 1;
    } else {
      console.log(`route-loaders: ${unsettled.length} route(s) with no loader, all in the ledger`);
    }
  }
}

export { compareToLedger, findUnsettled, routeStatus };
