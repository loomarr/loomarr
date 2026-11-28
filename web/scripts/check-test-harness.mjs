// The test-harness ledger (#4706). A universal package's component test mounts with `renderUi` from
// @loomarr/test-support (or Testing Library's `render`), not with a React root it creates itself.
// This counts the `createRoot(...)` calls in each test file.
//
// The committed ledger (test-harness.json) is a ratchet, like primitive-bypass.json: the check fails
// when a file's count rises or a file is new to it, and `--write` only lowers counts and drops
// files. With no ledger yet, `--write` records the current state.
import { existsSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { parse } from "@babel/parser";

const webRoot = fileURLToPath(new URL("../", import.meta.url));
const ledgerPath = join(webRoot, "test-harness.json");

const LEDGER_NOTE =
  "createRoot calls left in tests, maintained by web/scripts/check-test-harness.mjs. Counts may only fall: mount with renderUi from @loomarr/test-support and run `pnpm test-harness:update`. Never raise one to pass the check.";

const isTestFile = (path) => /\.test\.tsx?$/.test(path) || /\/tests\/.*\.tsx?$/.test(path);

const testFiles = (directory) =>
  readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    if (["node_modules", "dist", "generated"].includes(entry.name)) return [];
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return testFiles(path);
    return isTestFile(path.split(sep).join("/")) ? [path] : [];
  });

const countRoots = (source, { jsx = true } = {}) => {
  // A `.ts` file reads `<T>value` as a type assertion, which the jsx plugin would refuse.
  const ast = parse(source, { sourceType: "module", plugins: jsx ? ["typescript", "jsx"] : ["typescript"] });
  let count = 0;
  const visit = (node) => {
    if (Array.isArray(node)) return node.forEach(visit);
    if (!node || typeof node.type !== "string") return;
    if (node.type === "CallExpression" && node.callee.name === "createRoot") count += 1;
    Object.values(node).forEach(visit);
  };
  visit(ast.program);
  return count;
};

const compare = (ledger, current) =>
  Object.entries(current)
    .filter(([file, count]) => count > (ledger[file] ?? 0))
    .map(([file, count]) => `${file}: ${count} createRoot call(s), ledger allows ${ledger[file] ?? 0}`);

const shrink = (ledger, current) =>
  Object.fromEntries(
    Object.entries(ledger)
      .filter(([file]) => current[file])
      .map(([file, count]) => [file, Math.min(count, current[file])]),
  );

const scan = () =>
  Object.fromEntries(
    ["apps", "packages"]
      .flatMap((root) => testFiles(join(webRoot, root)))
      .map((path) => [
        relative(webRoot, path).split(sep).join("/"),
        countRoots(readFileSync(path, "utf8"), { jsx: path.endsWith(".tsx") }),
      ])
      .filter(([, count]) => count > 0)
      .sort(([a], [b]) => a.localeCompare(b)),
  );

const save = (files) =>
  writeFileSync(ledgerPath, `${JSON.stringify({ note: LEDGER_NOTE, files }, null, 2)}\n`);

const main = () => {
  const current = scan();
  const write = process.argv.includes("--write");
  if (!existsSync(ledgerPath)) {
    if (!write) throw new Error("test-harness.json is missing; run `pnpm test-harness:update`");
    save(current);
    return;
  }
  const ledger = JSON.parse(readFileSync(ledgerPath, "utf8")).files;
  const failures = compare(ledger, current);
  if (failures.length > 0) {
    console.error(`${failures.join("\n")}\nMount with renderUi from @loomarr/test-support.`);
    process.exitCode = 1;
    return;
  }
  if (write) save(shrink(ledger, current));
};

if (process.argv[1] === fileURLToPath(import.meta.url)) main();

export { compare, countRoots, shrink };
