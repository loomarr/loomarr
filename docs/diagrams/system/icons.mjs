// Builds the diagram icon set (docs/diagrams/icons/) from Lucide, the app's icon library. Each icon
// is written once per role and carries its own light and dark stroke: an SVG loaded as an image
// still honours prefers-color-scheme, so icons switch theme with the diagram. Colours are role
// tokens (web/packages/design-system/src/tokens/tokens.ts).
//
// The icons are committed, so rendering needs no Node packages. Run this only to add an icon:
//   LUCIDE_ICONS=<web>/node_modules/lucide-react/dist/esm/icons node docs/diagrams/system/icons.mjs
import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";

const LUCIDE = process.env.LUCIDE_ICONS;
if (!LUCIDE) throw new Error("LUCIDE_ICONS must name lucide-react's dist/esm/icons directory");
const OUT = path.join(import.meta.dirname, "..", "icons");

const ROLES = {
  loomarr: ["#795000", "#FFB020"], // action.primary
  audience: ["#08657A", "#4CC9E8"], // state.info
  service: ["#515866", "#8B93A3"], // content.secondary
};
const ICONS = {
  loomarr: [
    "calendar-clock",
    "clapperboard",
    "cpu",
    "list-checks",
    "list-plus",
    "list-video",
    "radio-tower",
    "search",
    "shield-check",
    "sparkles",
    "workflow",
  ],
  audience: ["git-pull-request", "monitor-play", "tv", "user-check", "users"],
  service: ["brain", "download", "file-video-camera", "film", "git-merge", "presentation", "server"],
};

mkdirSync(OUT, { recursive: true });
for (const [role, names] of Object.entries(ICONS)) {
  const [light, dark] = ROLES[role];
  for (const name of names) {
    const { __iconNode } = await import(`${LUCIDE}/${name}.mjs`);
    const body = __iconNode
      .map(([tag, attrs]) => {
        const a = Object.entries(attrs)
          .filter(([k]) => k !== "key")
          .map(([k, v]) => `${k}="${v}"`)
          .join(" ");
        return `<${tag} ${a}/>`;
      })
      .join("");
    const svg =
      `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` +
      `<style>*{stroke:${light}}@media (prefers-color-scheme:dark){*{stroke:${dark}}}</style>${body}</svg>\n`;
    writeFileSync(path.join(OUT, `${role}-${name}.svg`), svg);
  }
}
