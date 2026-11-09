import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// The motion rule (#2789, decided 2026-11-08): the product moves in three ways only — the tune,
// sheets sliding in from their edge over 200ms, and 120ms fades. Hover, focus and press change
// colour or border, never position or size: no scale, lift, nudge or bounce. This scans the web
// app and every package's source for a transform that animates, so a new one fails here rather
// than in review.
const APP = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const WEB = join(APP, "..", "..");

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
];

// Keyframes whose transform is the effect, not a movement: the TV snow's grain (entry moments only).
const ALLOWED_KEYFRAMES = new Set(["tv-snow"]);

const SKIP_DIRS = new Set(["node_modules", "dist", "generated", "test"]);

const sourceFiles = (dir: string): string[] =>
  readdirSync(dir).flatMap((entry) => {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) return SKIP_DIRS.has(entry) ? [] : sourceFiles(path);
    return /\.(tsx?|css)$/.test(entry) && !/\.(test|stories)\./.test(entry) ? [path] : [];
  });

const roots = [
  join(APP, "src"),
  ...readdirSync(join(WEB, "packages"))
    .map((name) => join(WEB, "packages", name, "src"))
    .filter((path) => {
      try {
        return statSync(path).isDirectory();
      } catch {
        return false;
      }
    }),
];

const files = roots.flatMap(sourceFiles).map((path) => ({
  path: relative(WEB, path),
  text: readFileSync(path, "utf8"),
}));

const RULES: { name: string; pattern: RegExp }[] = [
  // `transition-all` animates transform too, so it hides a scale or nudge behind a colour change.
  { name: "a transform or all-property transition class", pattern: /\btransition-(?:transform|all)\b/ },
  // A transform utility behind an interaction variant: a hover lift, a focus scale, a press shrink, an
  // arrow nudge. A state (`group-data-[panel-open]:rotate-180`) is not an interaction and may flip.
  {
    name: "a transform on hover, focus or press",
    pattern:
      /(?:^|[\s"'`])(?:[\w/[\]-]*(?:hover|focus|focus-visible|focus-within|active|pressed)[\w/-]*:)+-?(?:translate-[xy]|scale(?:-[xy])?|rotate)-/m,
  },
  // A native style that scales: the old press shrink and focus grow lived here.
  { name: "a scale transform in a style", pattern: /\{\s*scale[XY]?\s*:/ },
  // A CSS or inline transition naming transform (or all).
  {
    name: "a transition that names transform",
    pattern: /transition(?:-property|Property)?["']?\s*:\s*["'`]?[^;"'`}]*\b(?:transform|all)\b/,
  },
];

const allowed = (path: string) => ALLOWED.some((prefix) => path.startsWith(prefix));

describe("motion rule (#2789)", () => {
  it("finds the source it scans", () => {
    expect(files.length).toBeGreaterThan(500);
    expect(files.some((f) => f.path === "apps/web/src/styles.css")).toBe(true);
  });

  it("animates no transform outside the tune and sheets", () => {
    const offenders = files
      .filter((file) => !allowed(file.path))
      .flatMap((file) =>
        RULES.filter((rule) => rule.pattern.test(file.text)).map((rule) => `${file.path}: ${rule.name}`),
      );
    expect(offenders).toEqual([]);
  });

  it("keeps keyframe transforms to the static's grain", () => {
    const offenders = files
      .filter((file) => file.path.endsWith(".css"))
      .flatMap((file) =>
        Array.from(file.text.matchAll(/@keyframes\s+([\w-]+)\s*\{((?:[^{}]*\{[^{}]*\})*[^{}]*)\}/g))
          .filter(
            ([, name, body]) => /\btransform\s*:/.test(body ?? "") && !ALLOWED_KEYFRAMES.has(name ?? ""),
          )
          .map(([, name]) => `${file.path}: @keyframes ${name}`),
      );
    expect(offenders).toEqual([]);
  });

  // The scan must catch what it exists for; a pattern that matches nothing proves nothing.
  it.each([
    ["group-hover:translate-x-0.5", "a transform on hover, focus or press"],
    ["hover:-translate-y-1", "a transform on hover, focus or press"],
    ["active:scale-95", "a transform on hover, focus or press"],
    ["group-focus-visible/unlock:scale-90", "a transform on hover, focus or press"],
    ["size-4 transition-transform", "a transform or all-property transition class"],
    ["transition-all duration-200", "a transform or all-property transition class"],
    ["transform: [{ scale: pressed ? 0.98 : 1 }]", "a scale transform in a style"],
    ['style={{ transition: "transform 200ms" }}', "a transition that names transform"],
    ["transition: transform 120ms ease;", "a transition that names transform"],
  ])("flags %s", (sample, rule) => {
    expect(RULES.filter((r) => r.pattern.test(` ${sample}`)).map((r) => r.name)).toContain(rule);
  });

  it.each([
    "group-data-[panel-open]:rotate-180",
    "hover:text-foreground transition-colors",
    "-translate-x-1/2",
    "transition: opacity var(--duration-base) ease-out",
    "transition: grid-template-rows 260ms",
  ])("allows %s", (sample) => {
    expect(RULES.filter((r) => r.pattern.test(` ${sample}`))).toEqual([]);
  });
});
