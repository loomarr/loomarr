// The web's Tailwind theme, derived from the semantic tokens in ./tokens.ts: one palette for every
// app and package. Each raw color below is a value the semantic dark theme or the brand contract
// already names, so a hex lives in exactly one place. `webThemeCss()` renders the Tailwind v4
// `@theme` variables the web app imports as `@loomarr/design-system/theme.css`; the generator
// (scripts/generate-web-theme.ts, `make fe-tokens`) writes it to generated/theme.css and CI fails on
// drift.

import { brandChroma, semanticThemes, spaceGrid, typography } from "../tokens";

const dark = semanticThemes.dark;

// --- Static scale — "the set" (neutrals) ---
const staticScale = {
  "static-950": dark.surface.canvas, // app background
  "static-900": dark.surface.raised, // cards, panels (the badge/tint surface)
  "static-800": dark.surface.elevated, // nested / hover surfaces
  "static-700": dark.border.decorative, // hairlines, dividers (decorative only, §2.3)
  "static-500": dark.action.disabled, // DISABLED-only + decorative glyphs (2.94:1 — fails for info text)
  "static-400": dark.content.secondary, // secondary text, placeholders
  "static-100": dark.content.primary, // body text
  "static-0": semanticThemes.light.surface.raised, // high-emphasis text
} as const;

const staticExtras = {
  "signal-400": dark.guide.reconciling, // hover/active amber (11.3:1 on card)
  "border-control": dark.border.control, // the boundary color for controls that need ≥3:1 on 950/900/800 (§2.3)
} as const;

// --- Broadcast accents (the brand chroma) ---
// `on` names the on-tint text stop where the base fails WCAG on the composited 15% tint; the
// contrast test validates each `on`/tint pairing (§2.1 badge rule).
const accents = {
  signal: { base: brandChroma[0], on: brandChroma[0] }, // brand & primary; AA everywhere
  onair: { base: brandChroma[5], on: dark.state.live }, // live / destructive; on-tint uses onair-300
  suggest: { base: brandChroma[4], on: dark.accent.suggest }, // the AI color; on-tint uses suggest-300
  tune: { base: brandChroma[3], on: brandChroma[3] }, // links, informational, "tuning"
  lock: { base: brandChroma[2], on: brandChroma[2] }, // success, "signal locked"
  caution: { base: brandChroma[1], on: brandChroma[1] }, // warnings, drift (dark text on solid fills)
} as const;

// The CSS colour for text drawn on an accent's own tint: its `-300` stop where it has one, else the
// accent itself (which already clears AA there). The monogram ident's letters (#3011).
const onTintVar = (name: keyof typeof accents): string =>
  accents[name].on === accents[name].base ? `var(--color-${name})` : `var(--color-${name}-300)`;

// Semantic aliases so shadcn primitives restyle without edits (§2.1).
const semanticAliases = {
  primary: "signal",
  destructive: "onair",
  success: "lock",
  warning: "caution",
  info: "tune",
} as const;

// Tint steps — alpha washes, not fixed hexes: color-mix(in srgb, <accent> N%, transparent) over the
// surface. One formula replaces six tint tokens (§2.1).
const tintSteps = [8, 12, 15, 30, 40] as const;

// The surface badges/tints composite over (for the contrast validation, §2.1).
const tintSurface = staticScale["static-900"];

// --- Typography (§2.2) ---
const webType = {
  fonts: {
    sans: typography.family.body.web,
    mono: typography.family.data.web,
  },
  // If it came from a machine it's mono: channel numbers, EPG times, badges, ids.
  //
  // `2xs` is the CAPTION step — mono metadata that rides alongside content: a clip duration, a tick
  // label, a pod's era/quality. Without it the scale bottoms out at 12px and components invent their
  // own off-scale smaller values (`text-[10px]`, `text-[11px]`, `text-[10.5px]`). One sanctioned step
  // prevents that; the `Caption` primitive is the only thing that should use it.
  size: { "2xs": 11, xs: 12, sm: 13, base: 14, md: 16, lg: 20, xl: 24, "2xl": 32 },
  leading: { body: 1.5, heading: 1.2 },
} as const;

// --- Space (4px grid), radius, motion (§2.3, §2.4) ---
// Only the grid: a half step has no `--spacing-N` name (a dot is not valid in one), and the app's
// Tailwind already sizes its half-step utilities from the base unit.
const webSpace = spaceGrid;
const webRadius = { sm: 4, md: 8, lg: 12 } as const;
const webRowHeight = { compact: 40 } as const; // tables (§2.3)
const webMotion = {
  duration: { fast: "120ms", base: "200ms" },
  ease: "cubic-bezier(0.16, 1, 0.3, 1)", // ease-out
} as const;

// coreColors is the raw palette (statics + accents + -300 stops) — the only colors theme.css emits.
// The shadcn semantic layer (background/card/primary/border/…) is owned by the app (apps/web
// styles.css), so the aliases stay OUT of theme.css to avoid a --color-primary collision.
const coreColors = (): Record<string, string> => {
  const out: Record<string, string> = { ...staticScale, ...staticExtras };
  for (const [name, a] of Object.entries(accents)) {
    out[name] = a.base;
    if (a.on !== a.base) out[`${name}-300`] = a.on; // the on-tint text stop (§2.1)
  }
  return out;
};

// webTokenSet is every web token as plain data, with the semantic aliases added to the colors:
// the reference the gallery's Tokens page lists.
const webTokenSet = () => {
  const colors = coreColors();
  for (const [alias, accent] of Object.entries(semanticAliases)) colors[alias] = accents[accent].base;
  return {
    colors,
    space: webSpace,
    radius: webRadius,
    typography: webType,
    motion: webMotion,
    tintSteps,
  };
};

const HEADER = "/* GENERATED by @loomarr/design-system — edit src/tokens, run `make fe-tokens`. */";

// webThemeCss renders Tailwind v4 `@theme` variables. Output is deterministic (stable ordering,
// trailing newline) so `make fe-tokens` diffs are empty in CI.
const webThemeCss = (): string => {
  const lines: string[] = [HEADER, "", "@theme {"];
  lines.push("  /* colors */");
  for (const [k, v] of Object.entries(coreColors())) lines.push(`  --color-${k}: ${v};`);
  lines.push("", "  /* accent tints — alpha washes over the surface (§2.1) */");
  for (const name of Object.keys(accents))
    for (const step of tintSteps)
      lines.push(
        `  --color-${name}-tint-${step}: color-mix(in srgb, var(--color-${name}) ${step}%, transparent);`,
      );
  lines.push("", "  /* typography */");
  lines.push(`  --font-sans: ${webType.fonts.sans};`);
  lines.push(`  --font-mono: ${webType.fonts.mono};`);
  for (const [k, v] of Object.entries(webType.size)) lines.push(`  --text-${k}: ${v}px;`);
  lines.push(`  --leading-body: ${webType.leading.body};`);
  lines.push(`  --leading-heading: ${webType.leading.heading};`);
  lines.push("", "  /* space, radius */");
  for (const [k, v] of Object.entries(webSpace)) lines.push(`  --spacing-${k}: ${v}px;`);
  for (const [k, v] of Object.entries(webRadius)) lines.push(`  --radius-${k}: ${v}px;`);
  lines.push(`  --row-compact: ${webRowHeight.compact}px;`);
  lines.push("", "  /* motion */");
  lines.push(`  --ease-out: ${webMotion.ease};`);
  for (const [k, v] of Object.entries(webMotion.duration)) lines.push(`  --duration-${k}: ${v};`);
  lines.push("}", "");
  return lines.join("\n");
};

export {
  accents,
  onTintVar,
  semanticAliases,
  staticExtras,
  staticScale,
  tintSteps,
  tintSurface,
  webMotion,
  webRadius,
  webRowHeight,
  webSpace,
  webThemeCss,
  webTokenSet,
  webType,
};
