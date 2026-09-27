// @ts-check
// PROTOTYPE for #1572: Starlight, re-themed from the design-system tokens, with two candidate navs.
// ORG=diataxis (default) or ORG=audience picks the sidebar.
import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";
import githubAlerts from "./src/remark-github-alerts.mjs";
import remarkDiagramPaths from "./src/remark-diagram-paths.mjs";

const base = "/loomarr";

const diataxis = [
  { label: "Get started", slug: "get-started" },
  {
    label: "Guides",
    items: [
      "guides/add-a-channel", "guides/tv-app", "guides/hardware-encoding", "guides/filler",
      "guides/watermarks", "guides/4k-hdr", "guides/backups", "guides/upgrade",
      "guides/troubleshooting",
    ],
  },
  {
    label: "Reference",
    items: ["reference/settings", "reference/api", "reference/cli", "reference/hardware"],
  },
  {
    label: "Explanation",
    items: [
      "explanation/how-loomarr-works", "explanation/playout", "explanation/curation",
      "explanation/privacy",
    ],
  },
  {
    label: "Contributing",
    collapsed: true,
    items: ["contributing", "contributing/ci", "contributing/releasing"],
  },
];

// Same pages, grouped by who is reading instead of by what kind of page it is.
const audience = [
  { label: "Get started", slug: "get-started" },
  {
    label: "Use Loomarr",
    items: [
      "guides/add-a-channel", "guides/tv-app", "guides/filler", "guides/watermarks",
      "explanation/how-loomarr-works", "explanation/curation", "explanation/privacy",
    ],
  },
  {
    label: "Run Loomarr",
    items: [
      "guides/hardware-encoding", "guides/4k-hdr", "guides/backups", "guides/upgrade",
      "guides/troubleshooting", "explanation/playout", "reference/settings", "reference/cli",
      "reference/hardware", "reference/api",
    ],
  },
  {
    label: "Build Loomarr",
    collapsed: true,
    items: ["contributing", "contributing/ci", "contributing/releasing"],
  },
];

export default defineConfig({
  site: "https://mantonx.github.io",
  base,
  publicDir: "../proto-docs/diagrams",
  markdown: { remarkPlugins: [githubAlerts, [remarkDiagramPaths, { base }]] },
  vite: { server: { fs: { allow: [".."] } } },
  integrations: [
    starlight({
      title: "Loomarr",
      description: "Describe a TV channel in a sentence. Loomarr builds it, plays it, and keeps it running.",
      social: [{ icon: "github", label: "GitHub", href: "https://github.com/loomarr/loomarr" }],
      customCss: ["@fontsource-variable/geist", "./src/styles/custom.css"],
      editLink: { baseUrl: "https://github.com/loomarr/loomarr/edit/main/docs/" },
      lastUpdated: false,
      sidebar: process.env.ORG === "audience" ? audience : diataxis,
    }),
  ],
});
