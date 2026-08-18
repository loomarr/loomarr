#!/usr/bin/env node
// Generate the Android client's design tokens from the SAME tokens.json the web app consumes.
//
// Why this exists: colours were hand-copied into Kotlin once and drifted immediately — the pairing
// screen shipped `#F59E0B`, a Tailwind amber, where Loomarr's signal colour is `#FFB020`. Nothing
// caught it, because a hex literal in Kotlin has no relationship to the design system. Generating
// the file means a colour either comes from the tokens or it does not exist.
//
// The web side does the same thing through its own codegen; this is the second renderer of one
// source, not a second source.

import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";

const root = resolve(import.meta.dirname, "..");
const tokensPath = resolve(root, "web/packages/tokens/generated/tokens.json");
const outPath = resolve(root, "android/app/src/main/java/tv/loomarr/tv/design/LoomarrTokens.kt");

const tokens = JSON.parse(readFileSync(tokensPath, "utf8"));

// `static-950` → `Static950`, `signal-400` → `Signal400`, `onair` → `Onair`.
const pascal = (name) =>
  name
    .split(/[-_]/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join("");

// "#FFB020" → 0xFFFFB020 (Compose wants ARGB; every token is opaque).
const argb = (hex) => {
  const clean = hex.replace("#", "").trim();
  if (!/^[0-9a-fA-F]{6}$/.test(clean)) return null;
  return `0xFF${clean.toUpperCase()}`;
};

const colorLines = Object.entries(tokens.colors ?? {})
  .map(([name, value]) => {
    const packed = argb(value);
    if (!packed) return null;
    return `    /** \`${name}\` — ${value} */\n    val ${pascal(name)} = Color(${packed})`;
  })
  .filter(Boolean)
  .join("\n\n");

const file = `// GENERATED FILE — DO NOT EDIT.
//
// Produced by scripts/gen-android-tokens.mjs from web/packages/tokens/generated/tokens.json, which
// is the one source of truth the web app also renders from. Edit the tokens, then run
// \`make android-tokens\`.
//
// A colour that is not in the design system cannot be referenced from here, which is the point: the
// first hand-written TV screen used a Tailwind amber (#F59E0B) in place of Loomarr's signal colour
// (#FFB020), and nothing could catch it.

package tv.loomarr.tv.design

import androidx.compose.ui.graphics.Color

/** Loomarr's design tokens, shared with the web client. */
object LoomarrTokens {
${colorLines}
}
`;

mkdirSync(dirname(outPath), { recursive: true });
writeFileSync(outPath, file);
console.log(`wrote ${outPath} (${Object.keys(tokens.colors ?? {}).length} colours)`);
