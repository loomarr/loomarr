import assert from "node:assert/strict";
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import { parse } from "@babel/parser";

// The motion rule (#2789, decided 2026-11-08): the product moves in three ways only — the tune,
// sheets sliding in from their edge over 200ms, and 120ms fades. Hover, focus and press change
// colour or border, never position or size: no scale, lift, nudge or bounce. This scans the web
// app and every package's source for a transform that animates, so a new one fails here rather
// than in review.
//
// TypeScript is parsed and only its string and template literals (class names, inline CSS) and
// object properties (native styles) are checked, so a comment that names a transform is not one.
// CSS is checked as text.
const WEB = fileURLToPath(new URL("..", import.meta.url));

// Where a transform is the motion itself. Each entry says why.
const ALLOWED = [
  // The tune: the channel-switch overlay and its signal-acquisition loader.
  "packages/ui/src/channel-switch-overlay/",
  "packages/design-system/src/loading/",
  // Sheets, which slide in from their edge.
  "packages/design-system/src/sheet/",
  "packages/ui/src/overlay/",
  // The brand ident's launch, an entry moment (login and the wizard), like the static.
  "packages/design-system/src/brand-launch/",
  // Not motion: the Apple TV stage's fixed render scale, a layout transform that never animates.
  "packages/design-system/src/viewport/",
];

// Keyframes whose transform is the effect, not a movement: the TV snow's grain (entry moments only).
const ALLOWED_KEYFRAMES = new Set(["tv-snow"]);

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

const INTERACTION_VARIANT = /hover|focus|active|pressed/;
const TRANSFORM_UTILITY = /^-?(?:translate-[xy]|scale(?:-[xy])?|rotate)-/;

// transformOnInteraction reads each class token as its variants and its utility
// (`group-hover:translate-x-0.5` is the variant `group-hover` and the utility `translate-x-0.5`).
// Splitting, not one pattern over the variant chain, keeps the check linear on any input.
const transformOnInteraction = (text) =>
  text.split(/[\s"'`]+/).some((token) => {
    const variants = token.split(":");
    const utility = variants.pop() ?? "";
    return TRANSFORM_UTILITY.test(utility) && variants.some((variant) => INTERACTION_VARIANT.test(variant));
  });

// Rules over text: a class string, an inline CSS string, or a stylesheet.
const TEXT_RULES = [
  // `transition-all` animates transform too, so it hides a scale or nudge behind a colour change.
  {
    name: "a transform or all-property transition class",
    test: (text) => /\btransition-(?:transform|all)\b/.test(text),
  },
  // A transform utility behind an interaction variant: a hover lift, a focus scale, a press shrink, an
  // arrow nudge. A state (`group-data-[panel-open]:rotate-180`) is not an interaction and may flip.
  { name: "a transform on hover, focus or press", test: transformOnInteraction },
  // A CSS transition naming transform (or all).
  {
    name: "a transition that names transform",
    test: (text) => /transition(?:-property)?\s*:\s*[^;"'`}]*\b(?:transform|all)\b/.test(text),
  },
];

const SCALE_KEYS = new Set(["scale", "scaleX", "scaleY"]);
const TRANSITION_KEYS = new Set(["transition", "transitionProperty"]);
const NAMES_TRANSFORM = /\b(?:transform|all)\b/;

const keyName = (key) =>
  key.type === "Identifier" ? key.name : key.type === "StringLiteral" ? key.value : null;

const literalText = (node) => {
  if (node?.type === "StringLiteral") return node.value;
  if (node?.type === "TemplateLiteral") return node.quasis.map((q) => q.value.cooked ?? "").join(" ");
  return null;
};

// tsViolations returns the rule each offending literal or property in a parsed program breaks.
const tsViolations = (program) => {
  const found = new Set();
  const visit = (node) => {
    if (!node || typeof node.type !== "string") return;
    const text = node.type === "TemplateLiteral" || node.type === "StringLiteral" ? literalText(node) : null;
    if (text !== null) for (const rule of TEXT_RULES) if (rule.test(text)) found.add(rule.name);
    if (node.type === "ObjectProperty" && !node.computed) {
      const name = keyName(node.key);
      // A native style that scales: the old press shrink and focus grow lived here.
      if (SCALE_KEYS.has(name)) found.add("a scale transform in a style");
      // An inline transition naming transform (or all).
      const value = literalText(node.value);
      if (TRANSITION_KEYS.has(name) && value !== null && NAMES_TRANSFORM.test(value)) {
        found.add("a transition that names transform");
      }
    }
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

const cssViolations = (text) => TEXT_RULES.filter((rule) => rule.test(text)).map((rule) => rule.name);

// JSX only for .tsx: in a .ts file `<T>value` is a type assertion, which the JSX plugin misreads.
const parseSource = (code, path) =>
  parse(code, {
    sourceType: "module",
    plugins: path.endsWith(".tsx") ? ["typescript", "jsx"] : ["typescript"],
  }).program;

const violations = (code, path) =>
  path.endsWith(".css") ? cssViolations(code) : tsViolations(parseSource(code, path));

const allowed = (path) => ALLOWED.some((prefix) => path.startsWith(prefix));

// Each file read once: TypeScript goes to the parser, CSS is matched as text.
const sources = roots.flatMap(sourceFiles).map((path) => {
  const code = readFileSync(join(WEB, path), "utf8");
  return { path, code };
});

describe("motion rule (#2789)", () => {
  it("finds the source it scans", () => {
    assert.ok(sources.length > 500, `only ${sources.length} sources found; the walk stopped finding them`);
    assert.ok(
      sources.some((s) => s.path === "apps/web/src/styles.css"),
      "the app stylesheet is not scanned",
    );
  });

  it("animates no transform outside the tune and sheets", () => {
    const offenders = sources
      .filter(({ path }) => !allowed(path))
      .flatMap(({ path, code }) => violations(code, path).map((rule) => `${path}: ${rule}`));
    assert.deepEqual(offenders, [], "hover, focus and press change colour or border, never position or size");
  });

  it("keeps keyframe transforms to the static's grain", () => {
    const offenders = sources
      .filter(({ path }) => path.endsWith(".css"))
      .flatMap(({ path, code }) =>
        Array.from(code.matchAll(/@keyframes\s+([\w-]+)\s*\{((?:[^{}]*\{[^{}]*\})*[^{}]*)\}/g))
          .filter(
            ([, name, body]) => /\btransform\s*:/.test(body ?? "") && !ALLOWED_KEYFRAMES.has(name ?? ""),
          )
          .map(([, name]) => `${path}: @keyframes ${name}`),
      );
    assert.deepEqual(offenders, []);
  });

  // The scan must catch what it exists for; a rule that matches nothing proves nothing.
  const flags = [
    ["x.tsx", `<i className="group-hover:translate-x-0.5" />`, "a transform on hover, focus or press"],
    ["x.tsx", `<i className="hover:-translate-y-1" />`, "a transform on hover, focus or press"],
    ["x.ts", `const c = cn("p-2", "active:scale-95");`, "a transform on hover, focus or press"],
    [
      "x.ts",
      // biome-ignore lint/suspicious/noTemplateCurlyInString: the fixture is source text holding a template literal
      "const c = `group-focus-visible/unlock:scale-90 ${tone}`;",
      "a transform on hover, focus or press",
    ],
    [
      "x.tsx",
      `<i className="size-4 transition-transform" />`,
      "a transform or all-property transition class",
    ],
    ["x.ts", `const c = "transition-all duration-200";`, "a transform or all-property transition class"],
    ["x.ts", "const s = { transform: [{ scale: pressed ? 0.98 : 1 }] };", "a scale transform in a style"],
    ["x.tsx", `<i style={{ transition: "transform 200ms" }} />`, "a transition that names transform"],
    ["x.css", "a { transition: transform 120ms ease; }", "a transition that names transform"],
  ];
  for (const [path, sample, rule] of flags) {
    it(`flags ${sample}`, () => {
      assert.ok(violations(sample, path).includes(rule), `expected "${rule}"`);
    });
  }

  const allows = [
    ["x.tsx", `<i className="group-data-[panel-open]:rotate-180" />`],
    ["x.tsx", `<i className="hover:text-foreground transition-colors" />`],
    ["x.tsx", `<i className="-translate-x-1/2" />`],
    ["x.ts", "// the old hover:scale-105 and transition-transform are gone\nconst c = 1;"],
    ["x.css", "a { transition: opacity var(--duration-base) ease-out; }"],
    ["x.css", "a { transition: grid-template-rows 260ms; }"],
  ];
  for (const [path, sample] of allows) {
    it(`allows ${sample}`, () => {
      assert.deepEqual(violations(sample, path), []);
    });
  }
});
