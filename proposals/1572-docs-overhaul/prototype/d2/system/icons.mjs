// Build the diagram icon set from Lucide (the app's icon library). Each icon is written once per
// role and carries its own light/dark stroke: an SVG loaded as an image still honours
// prefers-color-scheme, so icons switch theme with the diagram. Colours are role tokens.
import { writeFileSync, mkdirSync } from "node:fs";

const LUCIDE = process.env.LUCIDE_ICONS; // .../lucide-react/dist/esm/icons
const ROLES = {
  loomarr: ["#795000", "#FFB020"], // action.primary
  audience: ["#08657A", "#4CC9E8"], // state.info
  service: ["#515866", "#8B93A3"], // content.secondary
};
const ICONS = {
  loomarr: ["sparkles", "list-plus", "calendar-clock", "radio-tower", "clapperboard", "cpu", "list-video"],
  audience: ["users", "tv", "monitor-play"],
  service: ["film", "brain", "download", "server", "file-video-camera", "presentation"],
};

mkdirSync("icons", { recursive: true });
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
      `<style>*{stroke:${light}}@media (prefers-color-scheme:dark){*{stroke:${dark}}}</style>${body}</svg>`;
    writeFileSync(`icons/${role}-${name}.svg`, svg);
  }
}
