// The Web legacy-usage ledger (#970 step 5). It counts, per production file under apps/web/src,
// the legacy presentation stack that the Web migration retires: Tailwind utility classes,
// class-name helpers (tailwind-merge, clsx), CVA, Base UI, the copied shadcn components in
// src/components/ui, and the legacy token package @loomarr/tokens. Tests, stories and test helpers
// are not production code and are not counted.
//
// The committed ledger (apps/web/legacy-usage.json) is a ratchet:
//   - the check fails when any count rises or a file with legacy usage is not in the ledger;
//   - it passes when counts fall, and `--write` then shrinks the ledger to match.
// `--write` only ever lowers counts and drops files. It never raises a count or adds a file, so
// regenerating cannot accept new debt; that takes a hand edit to the JSON, visible in review (a
// renamed file is the legitimate case). With no ledger yet, `--write` records the current state.
import { existsSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { parse } from "@babel/parser";

const appRoot = fileURLToPath(new URL("../apps/web/", import.meta.url));
const sourceRoot = join(appRoot, "src");
const legacyUiRoot = join(sourceRoot, "components", "ui");
const ledgerPath = join(appRoot, "legacy-usage.json");

// Every measure, in report order. A file's entry lists only its non-zero measures.
const MEASURES = ["tailwindClasses", "tailwindMerge", "clsx", "cva", "baseUi", "legacyUi", "legacyTokens"];

const isToolingFile = (path) =>
  path.includes(`${sep}test${sep}`) ||
  /\.(?:test|stories)\.(?:ts|tsx)$/.test(path) ||
  path.endsWith("routeTree.gen.ts");

const productionFiles = (directory) =>
  readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return productionFiles(path);
    return /\.(?:ts|tsx|css)$/.test(entry.name) && !isToolingFile(path) ? [path] : [];
  });

// The copied component a path belongs to: the first directory under src/components/ui, "" for
// the directory's own root (its barrel), or null outside it. A module path names its component by
// its first segment (`ui/button`); a file names it by its directory (`ui/button/button.tsx`), and a
// file at the root (`ui/index.ts`) belongs to none. An import inside one copied component
// (button/index.ts → ./button) is that component's own wiring, not a consumer of it.
const legacyComponentOf = (path, isFile) => {
  const inside = relative(legacyUiRoot, path);
  if (inside.startsWith("..") || inside.startsWith(sep)) return null;
  const segments = inside === "" ? [] : inside.split(sep);
  if (isFile) segments.pop();
  return segments[0] ?? "";
};

// importCategory names the measure an import specifier counts toward, or null.
const importCategory = (specifier, file) => {
  const under = (name) => specifier === name || specifier.startsWith(`${name}/`);
  if (under("tailwind-merge")) return "tailwindMerge";
  if (under("clsx")) return "clsx";
  if (under("class-variance-authority")) return "cva";
  if (under("@base-ui/react")) return "baseUi";
  if (under("@loomarr/tokens")) return "legacyTokens";
  let target = null;
  if (under("@/components/ui")) target = join(sourceRoot, specifier.slice(2));
  else if (specifier.startsWith(".")) target = resolve(dirname(file), specifier);
  if (target === null) return null;
  const component = legacyComponentOf(target, false);
  if (component === null) return null;
  return component !== "" && component === legacyComponentOf(file, true) ? null : "legacyUi";
};

const CLASS_HELPERS = new Set(["cn", "cx", "clsx", "twMerge", "cva"]);
const NON_CHILD_KEYS = new Set([
  "loc",
  "start",
  "end",
  "extra",
  "leadingComments",
  "trailingComments",
  "innerComments",
]);

const classTokens = (text) => text.split(/\s+/).filter(Boolean).length;

const keyName = (key) => {
  if (!key) return undefined;
  if (key.type === "Identifier" || key.type === "JSXIdentifier") return key.name;
  if (key.type === "StringLiteral") return key.value;
  return undefined;
};

const isClassKey = (key) => keyName(key) === "className" || keyName(key) === "class";

// countClasses counts the class tokens in string literals that reach a class name: a className
// (or class) attribute or property value, or an argument of a class helper (cn, cx, clsx,
// twMerge, cva). Object keys, comparison operands, conditions, cva's defaultVariants and the
// selectors of its compoundVariants are not classes. The arguments of any other call inside a
// class name (buttonVariants({ variant: "ghost" })) are variant choices, not classes.
const countClasses = (node, inClass = false) => {
  if (!node || typeof node !== "object") return 0;
  if (Array.isArray(node)) return node.reduce((sum, child) => sum + countClasses(child, inClass), 0);
  switch (node.type) {
    case "StringLiteral":
      return inClass ? classTokens(node.value) : 0;
    case "TemplateLiteral":
      return (
        (inClass ? node.quasis.reduce((sum, quasi) => sum + classTokens(quasi.value.cooked ?? ""), 0) : 0) +
        countClasses(node.expressions, inClass)
      );
    case "JSXAttribute":
      return countClasses(node.value, inClass || isClassKey(node.name));
    case "ObjectProperty": {
      if (isClassKey(node.key)) return countClasses(node.value, true);
      const name = keyName(node.key);
      if (inClass && name === "defaultVariants") return 0;
      return countClasses(node.value, inClass && name !== "compoundVariants");
    }
    case "CallExpression":
      if (node.callee.type === "Identifier" && CLASS_HELPERS.has(node.callee.name)) {
        return countClasses(node.arguments, true);
      }
      return countClasses(node.callee) + countClasses(node.arguments);
    case "BinaryExpression":
      return countClasses(node.left) + countClasses(node.right);
    case "ConditionalExpression":
      return (
        countClasses(node.test) +
        countClasses(node.consequent, inClass) +
        countClasses(node.alternate, inClass)
      );
    default: {
      // Type positions never hold classes: `"primary" | "ghost"` is a variant name.
      const childInClass = inClass && !/^TS.*(?:Type|Keyword)/.test(node.type);
      let sum = 0;
      for (const [key, child] of Object.entries(node)) {
        if (!NON_CHILD_KEYS.has(key) && child && typeof child === "object")
          sum += countClasses(child, childInClass);
      }
      return sum;
    }
  }
};

// importSpecifiers returns every module a source imports: static and type-only imports,
// re-exports, and dynamic import() of a string literal.
const importSpecifiers = (node, found = []) => {
  if (!node || typeof node !== "object") return found;
  if (Array.isArray(node)) {
    for (const child of node) importSpecifiers(child, found);
    return found;
  }
  if (
    (node.type === "ImportDeclaration" ||
      node.type === "ExportAllDeclaration" ||
      node.type === "ExportNamedDeclaration") &&
    node.source
  ) {
    found.push(node.source.value);
  } else if (
    node.type === "CallExpression" &&
    node.callee.type === "Import" &&
    node.arguments[0]?.type === "StringLiteral"
  ) {
    found.push(node.arguments[0].value);
  }
  for (const [key, child] of Object.entries(node)) {
    if (!NON_CHILD_KEYS.has(key) && child && typeof child === "object") importSpecifiers(child, found);
  }
  return found;
};

const withoutZeros = (counts) =>
  Object.fromEntries(
    MEASURES.filter((measure) => counts[measure] > 0).map((measure) => [measure, counts[measure]]),
  );

// measureSource counts one file's legacy usage. `file` is its absolute path, which resolves
// relative imports and decides whether a components/ui import is the component's own wiring.
const measureSource = (source, file) => {
  const counts = Object.fromEntries(MEASURES.map((measure) => [measure, 0]));
  if (file.endsWith(".css")) {
    for (const match of source.matchAll(/@import\s+(?:url\()?["']([^"']+)["']/g)) {
      const category = importCategory(match[1], file);
      if (category) counts[category] += 1;
    }
    return withoutZeros(counts);
  }
  const program = parse(source, {
    sourceType: "module",
    plugins: file.endsWith(".tsx") ? ["typescript", "jsx"] : ["typescript"],
  }).program;
  counts.tailwindClasses = countClasses(program);
  for (const specifier of importSpecifiers(program)) {
    const category = importCategory(specifier, file);
    if (category) counts[category] += 1;
  }
  return withoutZeros(counts);
};

// measureApp returns { "src/…": counts } for every production file with legacy usage.
const measureApp = (root = sourceRoot) => {
  const files = {};
  for (const file of productionFiles(root).sort()) {
    const counts = measureSource(readFileSync(file, "utf8"), file);
    if (Object.keys(counts).length > 0) files[relative(appRoot, file).split(sep).join("/")] = counts;
  }
  return files;
};

// compareToLedger lists what the ledger does not allow (rises and unlisted files) and whether
// the ledger could shrink.
const compareToLedger = (current, ledger) => {
  const violations = [];
  let canShrink = false;
  for (const [file, counts] of Object.entries(current)) {
    const allowed = ledger[file];
    if (!allowed) {
      violations.push(`${file}: new file with legacy usage (${formatCounts(counts)})`);
      continue;
    }
    for (const measure of MEASURES) {
      const now = counts[measure] ?? 0;
      const was = allowed[measure] ?? 0;
      if (now > was) violations.push(`${file}: ${measure} rose from ${was} to ${now}`);
      else if (now < was) canShrink = true;
    }
  }
  for (const file of Object.keys(ledger)) {
    if (!current[file]) canShrink = true;
  }
  return { violations, canShrink };
};

// shrinkLedger lowers each ledger count to the current count and drops emptied files. It never
// raises a count or adds a file.
const shrinkLedger = (current, ledger) => {
  const files = {};
  for (const [file, allowed] of Object.entries(ledger)) {
    const counts = Object.fromEntries(
      MEASURES.map((measure) => [measure, Math.min(allowed[measure] ?? 0, current[file]?.[measure] ?? 0)]),
    );
    const kept = withoutZeros(counts);
    if (Object.keys(kept).length > 0) files[file] = kept;
  }
  return files;
};

const totals = (files) =>
  Object.fromEntries(
    MEASURES.map((measure) => [
      measure,
      Object.values(files).reduce((sum, counts) => sum + (counts[measure] ?? 0), 0),
    ]),
  );

const formatCounts = (counts) =>
  MEASURES.filter((measure) => counts[measure])
    .map((measure) => `${measure} ${counts[measure]}`)
    .join(", ");

const LEDGER_NOTE =
  "Web legacy-usage ledger (#970 step 5), maintained by web/scripts/check-legacy-usage.mjs. Counts may only fall: `pnpm legacy:update` shrinks them. Never raise one to pass the check.";

const writeLedger = (files) =>
  writeFileSync(ledgerPath, `${JSON.stringify({ note: LEDGER_NOTE, files }, null, 2)}\n`);

const isMain = process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1];
if (isMain) {
  const current = measureApp();
  const hasLedger = existsSync(ledgerPath);
  const ledger = hasLedger ? JSON.parse(readFileSync(ledgerPath, "utf8")).files : {};
  const summary = `${Object.keys(current).length} files · ${formatCounts(totals(current))}`;
  if (process.argv.includes("--write")) {
    const files = hasLedger ? shrinkLedger(current, ledger) : current;
    writeLedger(files);
    console.log(`legacy-usage: ledger ${hasLedger ? "shrunk" : "recorded"} — ${summary}`);
  } else {
    const { violations, canShrink } = compareToLedger(current, ledger);
    if (violations.length > 0) {
      console.error("Legacy Web usage rose past apps/web/legacy-usage.json (#970):");
      for (const violation of violations) console.error(`  ${violation}`);
      console.error("Build new UI on @loomarr/ui and @loomarr/design-system instead of the legacy stack.");
      process.exitCode = 1;
    } else {
      console.log(`legacy-usage: within the ledger — ${summary}`);
      if (canShrink) console.log("legacy-usage: usage fell; run `pnpm legacy:update` to shrink the ledger.");
    }
  }
}

export { compareToLedger, countClasses, importCategory, measureApp, measureSource, shrinkLedger, totals };
