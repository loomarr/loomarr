// @ts-check
import path from "node:path";
import { defineConfig } from "astro/config";
import { unified } from "@astrojs/markdown-remark";
import starlight from "@astrojs/starlight";
import remarkDiagramPaths from "./src/remark-diagram-paths.mjs";
import remarkGithubAlerts from "./src/remark-github-alerts.mjs";
import remarkMdLinks from "./src/remark-md-links.mjs";
import redirects from "./src/redirects.json" with { type: "json" };
import { PUBLISHED } from "./src/site-pages.mjs";

// Published to GitHub Pages at https://mantonx.github.io/loomarr/. `base` must match the repo
// name or every internal link 404s on the deployed site while working perfectly in dev.
const base = "/loomarr";
const repoUrl = "https://github.com/loomarr/loomarr";

export default defineConfig({
  site: "https://mantonx.github.io",
  base,
  // Moved pages keep their old URLs (src/redirects.json, guarded by redirects.test.mjs).
  redirects: Object.fromEntries(
    Object.entries(redirects).map(([from, to]) => [from, `${base}${to}/`]),
  ),
  // Serve the canonical diagram tree directly. The remark adapters below map repository-relative
  // Markdown paths onto this public tree; no copied diagram directory can drift.
  publicDir: "../docs/diagrams",
  markdown: {
    processor: unified({
      remarkPlugins: [
        remarkGithubAlerts,
        [remarkDiagramPaths, { base }],
        [remarkMdLinks, { base, docsRoot: path.resolve("../docs"), include: PUBLISHED, repoUrl }],
      ],
    }),
  },
  // The docs live outside this directory (see src/content.config.mjs), so Vite needs explicit
  // permission to read them. Without it the build fails on the first file with a scarcely
  // related "outside of Vite serving allow list" error.
  vite: {
    server: { fs: { allow: [".."] } },
  },
  integrations: [
    starlight({
      title: "Loomarr",
      description: "Always something on. Describe a TV channel in a sentence; Loomarr builds it, plays it, and keeps it running.",
      social: [{ icon: "github", label: "GitHub", href: repoUrl }],
      // Only the landing page has a hero; its drawing replaces Starlight's image-beside-title.
      components: { Hero: "./src/components/Hero.astro" },
      customCss: [
        "@fontsource-variable/geist",
        "@fontsource-variable/geist-mono",
        "./src/styles/custom.css",
      ],
      editLink: { baseUrl: `${repoUrl}/edit/main/docs/` },
      // Diátaxis: one kind of page per section (#1572). Each section is one folder in docs/.
      // ⚠ `autogenerate` must sit INSIDE `items`, not beside `label`. Starlight v0.39 removed the
      // labelled-autogenerate shorthand; the old form is a config error that fails the build.
      sidebar: [
        { label: "Get started", slug: "get-started" },
        {
          label: "Guides",
          items: [
            "guides/install-docker",
            "guides/connect-services",
            "guides/add-a-channel",
            "guides/filler",
            "guides/hardware-encoding",
            "guides/monitoring",
            "guides/backup",
            "guides/upgrade",
            "guides/troubleshooting",
          ],
        },
        {
          label: "Reference",
          items: ["reference/settings", "reference/make"],
        },
        {
          label: "Explanation",
          items: ["explanation/how-loomarr-works", "explanation/curation"],
        },
        {
          label: "Contributing",
          collapsed: true,
          items: [
            "contributing",
            { autogenerate: { directory: "contributing" } },
            {
              label: "Design",
              collapsed: true,
              items: [
                { label: "System design", slug: "design" },
                "programming-design",
                "config-design",
                "frontend-design",
              ],
            },
          ],
        },
      ],
    }),
  ],
});
