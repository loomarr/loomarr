// The design-system primitive ledger (#2147 Wave 4). The design system owns Dialog, Table,
// Combobox/Select, Tooltip, DropdownMenu and Stack; app and package code outside it must use them
// rather than the library underneath or a hand-built copy. This counts, per production file, the
// ways code sidesteps a primitive:
//
//   rawBaseUi    imports of @base-ui/react or @radix-ui (the design system wraps them)
//   rnModal      `Modal` imported from react-native (the design system's Dialog is the modal)
//   rawDialog    role="dialog" / role="alertdialog" / aria-modal written by hand
//   rawTable     a <table> element or role="table" / "grid" written by hand
//   rawCombobox  role="combobox", or a <select> / <datalist> element
//   rawTooltip   role="tooltip" written by hand
//   rawMenu      role="menu" written by hand
//   flexStack    a className that makes an element a flex container (the design system's Stack)
//
// The committed ledger (primitive-bypass.json) is a ratchet, like legacy-usage.json:
//   - the check fails when any count rises or a file with a bypass is not in the ledger;
//   - it passes when counts fall, and `--write` then shrinks the ledger to match.
// `--write` only ever lowers counts and drops files; adding debt takes a hand edit to the JSON,
// visible in review. With no ledger yet, `--write` records the current state.
//
// The design-system package itself is the one place these belong, so it is not scanned. Tests,
// stories and test helpers are not production code and are not counted.
import { existsSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { parse } from "@babel/parser";

const webRoot = fileURLToPath(new URL("../", import.meta.url));
const ledgerPath = join(webRoot, "primitive-bypass.json");

// Where production source lives. packages/design-system is left out on purpose.
const SCAN_ROOTS = [
  "apps/web/src",
  "apps/mobile/app",
  "apps/mobile/src",
  "apps/tv/src",
  ...readdirSync(join(webRoot, "packages"), { withFileTypes: true })
    .filter((entry) => entry.isDirectory() && entry.name !== "design-system")
    .map((entry) => `packages/${entry.name}/src`),
];

const MEASURES = [
  "rawBaseUi",
  "rnModal",
  "rawDialog",
  "rawTable",
  "rawCombobox",
  "rawTooltip",
  "rawMenu",
  "flexStack",
];

const ROLE_MEASURE = {
  dialog: "rawDialog",
  alertdialog: "rawDialog",
  table: "rawTable",
  grid: "rawTable",
  combobox: "rawCombobox",
  tooltip: "rawTooltip",
  menu: "rawMenu",
};

const ELEMENT_MEASURE = { table: "rawTable", select: "rawCombobox", datalist: "rawCombobox" };

const NON_CHILD_KEYS = new Set([
  "loc",
  "start",
  "end",
  "extra",
  "leadingComments",
  "trailingComments",
  "innerComments",
]);

const isToolingFile = (path) =>
  path.includes(`${sep}test${sep}`) ||
  path.includes(`${sep}tests${sep}`) ||
  path.includes(`${sep}generated${sep}`) ||
  /\.(?:test|stories|type)\.(?:ts|tsx)$/.test(path) ||
  path.endsWith("routeTree.gen.ts");

const productionFiles = (directory) => {
  if (!existsSync(directory)) return [];
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return entry.name === "node_modules" ? [] : productionFiles(path);
    return /\.(?:ts|tsx)$/.test(entry.name) && !isToolingFile(path) ? [path] : [];
  });
};

const isLibrary = (specifier, name) => specifier === name || specifier.startsWith(`${name}/`);

const flexTokens = (text) => text.split(/\s+/).some((token) => token === "flex" || token === "inline-flex");

const attributeText = (attribute) => {
  const value = attribute.value;
  if (!value) return "";
  if (value.type === "StringLiteral") return value.value;
  if (value.type === "JSXExpressionContainer") {
    const expression = value.expression;
    if (expression.type === "StringLiteral") return expression.value;
    if (expression.type === "TemplateLiteral") {
      return expression.quasis.map((quasi) => quasi.value.cooked ?? "").join(" ");
    }
  }
  return "";
};

// classNameFlexes reports whether any string reaching a className attribute (a literal, a template
// or an argument of a helper such as cn) names a flex token.
const classNameFlexes = (node) => {
  if (!node || typeof node !== "object") return false;
  if (Array.isArray(node)) return node.some(classNameFlexes);
  if (node.type === "StringLiteral") return flexTokens(node.value);
  if (node.type === "TemplateLiteral") {
    return (
      node.quasis.some((quasi) => flexTokens(quasi.value.cooked ?? "")) || classNameFlexes(node.expressions)
    );
  }
  if (node.type === "ObjectExpression" || node.type === "BinaryExpression") return false;
  for (const [key, child] of Object.entries(node)) {
    if (!NON_CHILD_KEYS.has(key) && child && typeof child === "object" && classNameFlexes(child)) return true;
  }
  return false;
};

const walk = (node, counts) => {
  if (!node || typeof node !== "object") return;
  if (Array.isArray(node)) {
    for (const child of node) walk(child, counts);
    return;
  }
  if (
    (node.type === "ImportDeclaration" ||
      node.type === "ExportAllDeclaration" ||
      node.type === "ExportNamedDeclaration") &&
    node.source
  ) {
    const specifier = node.source.value;
    if (isLibrary(specifier, "@base-ui/react") || isLibrary(specifier, "@radix-ui")) counts.rawBaseUi += 1;
    if (
      specifier === "react-native" &&
      node.type === "ImportDeclaration" &&
      node.importKind !== "type" &&
      node.specifiers.some(
        (specifierNode) =>
          specifierNode.type === "ImportSpecifier" &&
          specifierNode.importKind !== "type" &&
          specifierNode.imported.name === "Modal",
      )
    ) {
      counts.rnModal += 1;
    }
  } else if (node.type === "ImportExpression" && node.source?.type === "StringLiteral") {
    const specifier = node.source.value;
    if (isLibrary(specifier, "@base-ui/react") || isLibrary(specifier, "@radix-ui")) counts.rawBaseUi += 1;
  } else if (node.type === "JSXOpeningElement") {
    if (node.name.type === "JSXIdentifier") {
      const measure = ELEMENT_MEASURE[node.name.name];
      if (measure) counts[measure] += 1;
    }
    let flexes = false;
    for (const attribute of node.attributes) {
      if (attribute.type !== "JSXAttribute" || attribute.name.type !== "JSXIdentifier") continue;
      const name = attribute.name.name;
      if (name === "role") {
        const measure = ROLE_MEASURE[attributeText(attribute)];
        if (measure) counts[measure] += 1;
      } else if (name === "aria-modal") {
        counts.rawDialog += 1;
      } else if (name === "className" && attribute.value && classNameFlexes(attribute.value)) {
        flexes = true;
      }
    }
    if (flexes) counts.flexStack += 1;
  }
  for (const [key, child] of Object.entries(node)) {
    if (!NON_CHILD_KEYS.has(key) && child && typeof child === "object") walk(child, counts);
  }
};

const withoutZeros = (counts) =>
  Object.fromEntries(
    MEASURES.filter((measure) => counts[measure] > 0).map((measure) => [measure, counts[measure]]),
  );

// measureSource counts one file's bypasses. `file` decides the parser plugins.
const measureSource = (source, file) => {
  const counts = Object.fromEntries(MEASURES.map((measure) => [measure, 0]));
  const program = parse(source, {
    sourceType: "module",
    plugins: file.endsWith(".tsx") ? ["typescript", "jsx"] : ["typescript"],
  }).program;
  walk(program, counts);
  return withoutZeros(counts);
};

// measureWorkspace returns { "apps/web/src/…": counts } for every production file with a bypass.
const measureWorkspace = () => {
  const files = {};
  for (const root of SCAN_ROOTS) {
    for (const file of productionFiles(join(webRoot, root)).sort()) {
      const counts = measureSource(readFileSync(file, "utf8"), file);
      if (Object.keys(counts).length > 0) files[relative(webRoot, file).split(sep).join("/")] = counts;
    }
  }
  return files;
};

const formatCounts = (counts) =>
  MEASURES.filter((measure) => counts[measure])
    .map((measure) => `${measure} ${counts[measure]}`)
    .join(", ");

// compareToLedger lists what the ledger does not allow (rises and unlisted files) and whether
// the ledger could shrink.
const compareToLedger = (current, ledger) => {
  const violations = [];
  let canShrink = false;
  for (const [file, counts] of Object.entries(current)) {
    const allowed = ledger[file];
    if (!allowed) {
      violations.push(`${file}: new bypass (${formatCounts(counts)})`);
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

const LEDGER_NOTE =
  "Design-system primitive ledger (#2147 Wave 4), maintained by web/scripts/check-primitive-bypass.mjs. Counts may only fall: `pnpm primitives:update` shrinks them. Never raise one to pass the check.";

const writeLedger = (files) =>
  writeFileSync(ledgerPath, `${JSON.stringify({ note: LEDGER_NOTE, files }, null, 2)}\n`);

const isMain = process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1];
if (isMain) {
  const current = measureWorkspace();
  const hasLedger = existsSync(ledgerPath);
  const ledger = hasLedger ? JSON.parse(readFileSync(ledgerPath, "utf8")).files : {};
  const summary = `${Object.keys(current).length} files · ${formatCounts(totals(current))}`;
  if (process.argv.includes("--write")) {
    const files = hasLedger ? shrinkLedger(current, ledger) : current;
    writeLedger(files);
    console.log(`primitive-bypass: ledger ${hasLedger ? "shrunk" : "recorded"} — ${summary}`);
  } else {
    const { violations, canShrink } = compareToLedger(current, ledger);
    if (violations.length > 0) {
      console.error("Design-system primitive bypasses rose past primitive-bypass.json (#2147):");
      for (const violation of violations) console.error(`  ${violation}`);
      console.error(
        "Use the @loomarr/design-system primitive (Dialog, Table, Menu, Tooltip, select, Stack) instead.",
      );
      process.exitCode = 1;
    } else {
      console.log(`primitive-bypass: within the ledger — ${summary}`);
      if (canShrink)
        console.log("primitive-bypass: usage fell; run `pnpm primitives:update` to shrink the ledger.");
    }
  }
}

export { compareToLedger, measureSource, shrinkLedger, totals };
