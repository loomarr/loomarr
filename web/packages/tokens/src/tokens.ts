// The Test Card design tokens — the SINGLE source of truth (frontend-design §2).
// No raw color/size literal may live outside this layer; a hex in a component is
// a review-blocking defect. The generator (generate.ts) emits three artifacts
// from this file — theme.css (Tailwind v4 @theme), a shared Tailwind preset
// (web now, NativeWind/Expo later), and tokens.json — and CI fails on drift.
//
// Values are the calibrated hex from frontend-design §2.1 (WCAG-AA-tuned SMPTE
// bars). NOTE: the design names OKLCH as the eventual on-disk form; emitting
// OKLCH in theme.css is a sanctioned follow-up (culori in the generator) — the
// rendered color is identical, so this is disclosed, not a silent deviation.

// --- Static scale — "the set" (neutrals) ---
export const staticScale = {
  "static-950": "#0B0C0E", // app background
  "static-900": "#131519", // cards, panels (the badge/tint surface)
  "static-800": "#1B1E24", // nested / hover surfaces
  "static-700": "#2A2E37", // hairlines, dividers (decorative only, §2.3)
  "static-500": "#5A6170", // DISABLED-only + decorative glyphs (2.94:1 — fails for info text)
  "static-400": "#8B93A3", // secondary text, placeholders
  "static-100": "#E7EAF0", // body text
  "static-0": "#FFFFFF", // high-emphasis text
} as const;

// Extra statics from the prototype (§2.1 / §2.3).
export const staticExtras = {
  "signal-400": "#FFC14D", // hover/active amber (11.3:1 on card)
  "border-control": "#61646B", // the boundary color for controls that need ≥3:1 (§2.3)
} as const;

// --- Broadcast accents (from the bars) ---
// `on` names the on-tint text stop where the base fails WCAG on the composited
// 15% tint; the generator VALIDATES each `on`/tint pairing (§2.1 badge rule).
export const accents = {
  signal: { base: "#FFB020", on: "#FFB020" }, // brand & primary; AA everywhere
  onair: { base: "#E5484D", on: "#E85A5F" }, // live / destructive; on-tint uses onair-300
  suggest: { base: "#D6409F", on: "#DC5BAC" }, // the AI color; on-tint uses suggest-300
  tune: { base: "#4CC9E8", on: "#4CC9E8" }, // links, informational, "tuning"
  lock: { base: "#3DD68C", on: "#3DD68C" }, // success, "signal locked"
  caution: { base: "#F5D90A", on: "#F5D90A" }, // warnings, drift (dark text on solid fills)
} as const;

// Semantic aliases so shadcn primitives restyle without edits (§2.1).
export const semanticAliases = {
  primary: "signal",
  destructive: "onair",
  success: "lock",
  warning: "caution",
  info: "tune",
} as const;

// Tint steps — alpha washes, not fixed hexes: color-mix(in srgb, <accent> N%,
// transparent) over the surface. One formula replaces six tint tokens (§2.1).
export const tintSteps = [8, 12, 15, 30, 40] as const;

// The surface badges/tints composite over (for the contrast validation, §2.1).
export const tintSurface = staticScale["static-900"];

// --- Typography (§2.2) ---
export const typography = {
  fonts: {
    sans: "Geist, ui-sans-serif, system-ui, sans-serif",
    mono: "'Geist Mono', ui-monospace, 'SF Mono', monospace",
  },
  // If it came from a machine it's mono: channel numbers, EPG times, badges, ids.
  size: { xs: 12, sm: 13, base: 14, md: 16, lg: 20, xl: 24, "2xl": 32 },
  leading: { body: 1.5, heading: 1.2 },
} as const;

// --- Space (4px grid), radius, motion (§2.3, §2.4) ---
export const space = { 1: 4, 2: 8, 3: 12, 4: 16, 5: 20, 6: 24, 7: 28, 8: 32 } as const;
export const radius = { sm: 4, md: 8, lg: 12 } as const;
export const rowHeight = { compact: 40 } as const; // tables (§2.3)
export const motion = {
  duration: { fast: "120ms", base: "200ms" },
  ease: "cubic-bezier(0.16, 1, 0.3, 1)", // ease-out
} as const;

export type Accent = keyof typeof accents;
export type Semantic = keyof typeof semanticAliases;
