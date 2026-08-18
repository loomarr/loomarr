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
//
// ⚠ A leading digit is moved to the END rather than prefixed, because Kotlin identifiers cannot
// start with one: the type scale's `2xs`/`2xl` would otherwise emit `val 2xl`, which does not
// compile. `2xl` becomes `Xl2`, keeping the name recognisable and legal.
const pascal = (name) => {
  const joined = name
    .split(/[-_]/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join("");
  const leadingDigits = joined.match(/^(\d+)(.*)$/);
  if (!leadingDigits) return joined;
  const [, digits, rest] = leadingDigits;
  return `${rest.charAt(0).toUpperCase()}${rest.slice(1)}${digits}`;
};

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
    return `        /** \`${name}\` — ${value} */\n        val ${pascal(name)} = Color(${packed})`;
  })
  .filter(Boolean)
  .join("\n\n");

// Spacing and radii are `dp`; type sizes are `sp` so they honour the viewer's font-scale setting.
const spaceLines = Object.entries(tokens.space ?? {})
  .map(([step, value]) => `        /** space.${step} */\n        val S${step} = ${value}.dp`)
  .join("\n\n");

const radiusLines = Object.entries(tokens.radius ?? {})
  .map(([name, value]) => `        /** radius.${name} */\n        val ${pascal(name)} = ${value}.dp`)
  .join("\n\n");

// ⚠ The web type scale is sized for a screen an arm's length away; a television is three metres
// off. Emitting these values RAW would make 14sp body text unreadable at that distance, so each is
// multiplied by TV_SCALE. The ratios between steps — what makes the scale a scale — are preserved;
// only the absolute size moves.
const TV_SCALE = 2;
const typeLines = Object.entries(tokens.typography?.size ?? {})
  .map(([name, value]) => {
    const scaled = Math.round(value * TV_SCALE);
    return (
      `        /** typography.size.${name} (${value}sp on web, ×${TV_SCALE} for 10-foot viewing) */\n` +
      `        val ${pascal(name)} = ${scaled}.sp`
    );
  })
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
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/** Loomarr's design tokens, shared with the web client. */
object LoomarrTokens {
    /** Palette. Identical values to the web client — same hex, same names. */
    object Color {
${colorLines}
    }

    /** Spacing scale, in dp. Same steps as the web client. */
    object Space {
${spaceLines}
    }

    /** Corner radii, in dp. */
    object Radius {
${radiusLines}
    }

    /**
     * Type scale, in sp.
     *
     * ⚠ Scaled ×${TV_SCALE} from the web values. A television is viewed from roughly three metres
     * rather than an arm's length, so web's 14sp body would be unreadable. The RATIOS between steps
     * are preserved — this moves the whole scale, it does not redesign it.
     */
    object Type {
${typeLines}

        /**
         * A pairing code, read across a room and typed into a phone.
         *
         * ⚠ NOT derived from the web scale, because the web has nothing like it — this is a value
         * a viewer must transcribe from several metres away, and the largest web step (${(tokens.typography?.size?.["2xl"] ?? 32)}sp)
         * is still a heading. It is stated here rather than inline in a screen so it stays one
         * decision instead of a literal someone later "tidies".
         */
        val Code = 88.sp
    }
}
`;

mkdirSync(dirname(outPath), { recursive: true });
writeFileSync(outPath, file);
const counts = [
  `${Object.keys(tokens.colors ?? {}).length} colours`,
  `${Object.keys(tokens.space ?? {}).length} spaces`,
  `${Object.keys(tokens.radius ?? {}).length} radii`,
  `${Object.keys(tokens.typography?.size ?? {}).length} type sizes`,
].join(", ");
console.log(`wrote ${outPath} (${counts})`);
