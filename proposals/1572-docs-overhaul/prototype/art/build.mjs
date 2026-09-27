// Build the brand illustrations: embed Geist and Geist Mono (OFL) so the SVGs render the same
// as an <img> on GitHub, the site and the README, then write *.svg next to each *.src.svg.
import { readFileSync, writeFileSync, readdirSync } from "node:fs";

const font = (family, file) =>
  `@font-face{font-family:'${family}';font-weight:100 900;src:url(data:font/woff2;base64,${readFileSync(file).toString("base64")}) format('woff2')}`;
const fonts =
  font("Geist", process.env.GEIST_WOFF2) + font("Geist Mono", process.env.GEIST_MONO_WOFF2);

for (const src of readdirSync(".").filter((f) => f.endsWith(".src.svg"))) {
  const out = src.replace(".src.svg", ".svg");
  const svg = readFileSync(src, "utf8").replace("/*FONTS*/", fonts);
  writeFileSync(out, svg);
  console.log(out, `${(svg.length / 1024).toFixed(0)} KB`);
}
