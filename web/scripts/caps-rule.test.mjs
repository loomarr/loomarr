import assert from "node:assert/strict";
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import { parse } from "@babel/parser";

// The caps rule (#2778, decided 2026-11-08): capitals are an accent, kept for channel numbers,
// clocks and provenance chips (the player's NOW/NEXT chips became plain "Now" and "Next" labels in
// variant J, #3449). Everything else is sentence case, and
// where capitals are drawn they come from the Text primitive's `shout`, not from a CSS class. This
// scans the web app and every package's source for a new use, so one fails here rather than in
// review.
//
// TypeScript is parsed: a `shout` JSX attribute, an uppercase `textTransform` (attribute or style
// property) and an `uppercase` class token in a string count, and a comment that names one does
// not. CSS is checked as text.
const WEB = fileURLToPath(new URL("..", import.meta.url));

// Where capitals are the accent the rule keeps. Each entry says what is in capitals there.
const ALLOWED = [
  // The Text primitive: `shout` is implemented here.
  "packages/design-system/src/text/",
  // The player band's channel number block ("CH").
  "packages/ui/src/watching-surface/player-chrome/channel-band/",
  // The Info sheet's "Up next" chip.
  "packages/ui/src/watching-surface/player-chrome/player-info/",
  // A channel page's number ("Ch 07").
  "apps/web/src/routes/_authed/channels/$id/route.tsx",
  // Provenance: the "Managed by Loomarr" line on a channel card.
  "apps/web/src/components/loomarr/channels/channel-card/",
  // A pairing code is typed and shown in capitals, as the device shows it.
  "apps/web/src/settings/pair-device/",
  "apps/web/src/settings/paired-devices/",
];

const SKIP_DIRS = new Set(["node_modules", "dist", "generated", "test"]);

const sourceFiles = (dir) =>
  readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return SKIP_DIRS.has(entry.name) ? [] : sourceFiles(path);
    const rel = relative(WEB, path).split(sep).join("/");
    return /\.(tsx?|css)$/.test(entry.name) && !/\.(test|stories)\./.test(entry.name) ? [rel] : [];
  });

const roots = [
  join(WEB, "apps", "web", "src"),
  ...readdirSync(join(WEB, "packages"), { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => join(WEB, "packages", entry.name, "src"))
    .filter((path) => existsSync(path)),
];

const SHOUT = "the shout prop";
const TRANSFORM = "an uppercase text transform";
const CLASS = "an uppercase class";

// `uppercase`, alone or behind variants (`md:uppercase`), as one token of a class string.
const uppercaseClass = (text) =>
  text.split(/[\s"'`]+/).some((token) => token.split(":").pop() === "uppercase");

const keyName = (key) =>
  key.type === "Identifier" ? key.name : key.type === "StringLiteral" ? key.value : null;

// `"uppercase" as const` is still the literal "uppercase".
const literalText = (node) => {
  if (node?.type === "TSAsExpression" || node?.type === "TSSatisfiesExpression")
    return literalText(node.expression);
  if (node?.type === "StringLiteral") return node.value;
  if (node?.type === "TemplateLiteral") return node.quasis.map((q) => q.value.cooked ?? "").join(" ");
  return null;
};

// A JSX attribute's value as text: `a="x"` and `a={"x"}` are both "x".
const attributeText = (value) =>
  literalText(value?.type === "JSXExpressionContainer" ? value.expression : value);

// `shout`, `shout={true}` and `shout={anything}` shout; only a literal `shout={false}` does not.
const shouts = (value) =>
  !(
    value?.type === "JSXExpressionContainer" &&
    value.expression.type === "BooleanLiteral" &&
    !value.expression.value
  );

// tsViolations returns the rule each offending attribute, property or literal in a program breaks.
const tsViolations = (program) => {
  const found = new Set();
  const visit = (node) => {
    if (!node || typeof node.type !== "string") return;
    if (node.type === "JSXAttribute" && node.name.type === "JSXIdentifier") {
      if (node.name.name === "shout" && shouts(node.value)) found.add(SHOUT);
      if (node.name.name === "textTransform" && attributeText(node.value) === "uppercase")
        found.add(TRANSFORM);
    }
    if (node.type === "ObjectProperty" && !node.computed) {
      if (keyName(node.key) === "textTransform" && literalText(node.value) === "uppercase")
        found.add(TRANSFORM);
    }
    const text = literalText(node);
    if (text !== null && uppercaseClass(text)) found.add(CLASS);
    for (const [key, value] of Object.entries(node)) {
      if (key === "loc" || key === "leadingComments" || key === "trailingComments" || key === "innerComments")
        continue;
      for (const child of Array.isArray(value) ? value : [value]) {
        if (child && typeof child === "object") visit(child);
      }
    }
  };
  visit(program);
  return [...found];
};

const cssViolations = (text) => (/text-transform\s*:\s*uppercase\b/.test(text) ? [TRANSFORM] : []);

// JSX only for .tsx: in a .ts file `<T>value` is a type assertion, which the JSX plugin misreads.
const parseSource = (code, path) =>
  parse(code, {
    sourceType: "module",
    plugins: path.endsWith(".tsx") ? ["typescript", "jsx"] : ["typescript"],
  }).program;

const violations = (code, path) =>
  path.endsWith(".css") ? cssViolations(code) : tsViolations(parseSource(code, path));

const allowed = (path) => ALLOWED.some((prefix) => path.startsWith(prefix));

const sources = roots.flatMap(sourceFiles).map((path) => {
  const code = readFileSync(join(WEB, path), "utf8");
  return { path, code };
});

describe("caps rule (#2778)", () => {
  it("finds the source it scans", () => {
    assert.ok(sources.length > 500, `only ${sources.length} sources found; the walk stopped finding them`);
    assert.ok(
      sources.some((s) => s.path === "apps/web/src/styles.css"),
      "the app stylesheet is not scanned",
    );
  });

  it("draws capitals only for channel numbers, clocks and provenance", () => {
    const offenders = sources
      .filter(({ path }) => !allowed(path))
      .flatMap(({ path, code }) => violations(code, path).map((rule) => `${path}: ${rule}`));
    assert.deepEqual(
      offenders,
      [],
      "sentence case everywhere else; a new use needs a line in ALLOWED saying why",
    );
  });

  // An entry that no longer holds capitals is a stale licence for whatever is added there next.
  it("allows no place that has stopped using capitals", () => {
    const stale = ALLOWED.filter(
      (prefix) =>
        !sources.some(({ path, code }) => path.startsWith(prefix) && violations(code, path).length > 0),
    );
    assert.deepEqual(stale, []);
  });

  // The scan must catch what it exists for; a rule that matches nothing proves nothing.
  const flags = [
    ["x.tsx", `<Text shout textRole="mono">Dead air</Text>`, SHOUT],
    ["x.tsx", `<Text shout={loud}>Paused</Text>`, SHOUT],
    ["x.tsx", `<Text textTransform="uppercase">Sources</Text>`, TRANSFORM],
    ["x.tsx", `<i style={{ letterSpacing: 1, textTransform: "uppercase" }} />`, TRANSFORM],
    ["x.ts", `const label = { textTransform: "uppercase" as const };`, TRANSFORM],
    ["x.tsx", `<p className="text-xs uppercase tracking-wide" />`, CLASS],
    ["x.ts", `const c = cn("font-mono", "md:uppercase");`, CLASS],
    ["x.css", ".eyebrow { text-transform: uppercase; }", TRANSFORM],
  ];
  for (const [path, sample, rule] of flags) {
    it(`flags ${sample}`, () => {
      assert.ok(violations(sample, path).includes(rule), `expected "${rule}"`);
    });
  }

  const allows = [
    ["x.tsx", `<Text shout={false} textRole="label">Sources</Text>`],
    ["x.tsx", `<Text textRole="label">Sources</Text>`],
    ["x.tsx", `<Field autoCapitalize="characters" />`],
    ["x.ts", `const s = { textTransform: "none" as const };`],
    ["x.ts", "// the old uppercase eyebrow and its shout are gone\nconst c = 1;"],
    ["x.tsx", `<p className="normal-case" />`],
    ["x.css", ".code { text-transform: none; }"],
  ];
  for (const [path, sample] of allows) {
    it(`allows ${sample}`, () => {
      assert.deepEqual(violations(sample, path), []);
    });
  }
});
