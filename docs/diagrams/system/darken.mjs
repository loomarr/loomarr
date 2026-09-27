// Post-render step for the Loomarr diagram system. D2 v0.7 cannot reference its theme slots from
// `style`, so role colours are written as their LIGHT token values and this step appends one
// dark-scheme rule per value. CSS attribute selectors outrank SVG presentation attributes, so no
// !important is needed and the light render is untouched.
import { readFileSync, writeFileSync } from "node:fs";

// light token value -> dark token value (web/packages/design-system/src/tokens/tokens.ts)
export const DARK = {
  "#17191D": "#E7EAF0", // content.primary
  "#515866": "#8B93A3", // content.secondary
  "#69717F": "#8B93A3", // content.muted
  "#747C8B": "#61646B", // border.control
  "#D2D6DE": "#2A2E37", // border.decorative
  "#E7EAF0": "#1B1E24", // surface.elevated
  "#F7F8FA": "#0B0C0E", // surface.canvas
  "#FFFFFF": "#131519", // surface.raised
  "#FFF3D6": "#282010", // surface.focus -> signal 12% over canvas
  "#795000": "#FFB020", // action.primary
  "#DAE8EB": "#1C3038", // stateSurface.info
  "#08657A": "#4CC9E8", // state.info
};

const rules = Object.entries(DARK)
  .flatMap(([light, dark]) =>
    ["fill", "stroke"].flatMap((attr) =>
      [light, light.toLowerCase()].map((v) => `[${attr}="${v}"]{${attr}:${dark}}`),
    ),
  )
  .join("");

for (const file of process.argv.slice(2)) {
  const svg = readFileSync(file, "utf8");
  if (svg.includes("data-loomarr-dark")) continue;
  const style = `<style data-loomarr-dark="">@media (prefers-color-scheme:dark){${rules}}</style>`;
  // Insert after the outer <svg ...> open tag so it scopes to this document.
  writeFileSync(file, svg.replace(/(<svg[^>]*>)/, `$1${style}`));
}
