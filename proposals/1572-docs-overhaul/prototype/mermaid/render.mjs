// Mermaid candidate: the same role palette as the D2 theme, from the same semantic tokens.
// Mermaid writes one theme per SVG, so each diagram renders twice (light, dark) and the page
// picks with <picture> + prefers-color-scheme.
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync, readdirSync } from "node:fs";

// GEIST_WOFF2: path to @fontsource-variable/geist's files/geist-latin-wght-normal.woff2
const woff2 = readFileSync(process.env.GEIST_WOFF2).toString("base64");

const themes = {
  light: {
    text: "#17191D", muted: "#515866", edge: "#69717F", card: "#FFFFFF", canvas: "#F7F8FA",
    brandFill: "#FFF3D6", brand: "#795000", svcFill: "#E7EAF0", svcStroke: "#747C8B",
    audFill: "#DAE8EB", aud: "#08657A", decorative: "#D2D6DE",
  },
  dark: {
    text: "#E7EAF0", muted: "#8B93A3", edge: "#8B93A3", card: "#131519", canvas: "#0B0C0E",
    brandFill: "#282010", brand: "#FFB020", svcFill: "#1B1E24", svcStroke: "#61646B",
    audFill: "#1C3038", aud: "#4CC9E8", decorative: "#2A2E37",
  },
};

const css = (t) => `
@font-face{font-family:Geist;font-weight:100 900;src:url(data:font/woff2;base64,${woff2}) format("woff2")}
svg{background:${t.card}}
.node.loomarr rect,.node.loomarr path{fill:${t.brandFill}!important;stroke:${t.brand}!important;stroke-width:2px!important}
.node.service rect,.node.service path{fill:${t.svcFill}!important;stroke:${t.svcStroke}!important;stroke-width:1px!important}
.node.audience rect,.node.audience path{fill:${t.audFill}!important;stroke:${t.aud}!important;stroke-width:2px!important}
.node .label, .node .nodeLabel{color:${t.text}!important;font-weight:600}
.cluster.boundary rect{fill:${t.canvas}!important;stroke:${t.brand}!important;stroke-dasharray:4 4}
.cluster.zone rect{fill:${t.card}!important;stroke:${t.decorative}!important}
.cluster-label .nodeLabel, .cluster-label span{color:${t.muted}!important;font-weight:400}
.flowchart-link{stroke:${t.edge}!important}
.edgeLabel, .edgeLabel p, .edgeLabel span{background:${t.card}!important;color:${t.text}!important}
.flowchart-link.edge-thickness-thick{stroke:${t.brand}!important;stroke-width:3px!important}
marker path{fill:${t.edge}!important;stroke:${t.edge}!important}
`;

for (const [name, t] of Object.entries(themes)) {
  const config = {
    theme: "base",
    fontFamily: "Geist, system-ui, sans-serif",
    themeVariables: {
      fontFamily: "Geist, system-ui, sans-serif",
      fontSize: "16px",
      background: t.card,
      primaryColor: t.svcFill,
      primaryTextColor: t.text,
      primaryBorderColor: t.svcStroke,
      lineColor: t.edge,
      textColor: t.text,
      clusterBkg: t.card,
      clusterBorder: t.decorative,
      edgeLabelBackground: t.card,
    },
    flowchart: { curve: "basis", padding: 16, nodeSpacing: 40, rankSpacing: 56, htmlLabels: true },
    themeCSS: css(t),
  };
  writeFileSync(`out/config-${name}.json`, JSON.stringify(config));
}

for (const src of readdirSync(".").filter((f) => f.endsWith(".mmd"))) {
  const base = src.replace(/\.mmd$/, "");
  for (const name of Object.keys(themes)) {
    execFileSync("docker", [
      "run", "--rm", "--network", "none", "-u", `${process.getuid()}:${process.getgid()}`,
      "-v", `${process.cwd()}:/data`, "minlag/mermaid-cli:11.12.0",
      "-i", src, "-o", `out/${base}-${name}.svg`, "-c", `out/config-${name}.json`,
      "-b", themes[name].card,
    ], { stdio: "inherit" });
  }
}
