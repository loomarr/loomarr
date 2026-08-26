// @ts-check
import { defineConfig } from "astro/config";
import { unified } from "@astrojs/markdown-remark";
import starlight from "@astrojs/starlight";
import remarkDiagramPaths from "./src/remark-diagram-paths.mjs";

// Published to GitHub Pages at https://mantonx.github.io/loomarr/. `base` must match the repo
// name or every internal link 404s on the deployed site while working perfectly in dev.
const base = "/loomarr";

export default defineConfig({
  site: "https://mantonx.github.io",
  base,
  // Serve the canonical diagram tree directly. The remark adapter below maps repository-relative
  // Markdown paths onto this public tree; no copied diagram directory can drift.
  publicDir: "../docs/diagrams",
  markdown: {
    processor: unified({ remarkPlugins: [[remarkDiagramPaths, { base }]] }),
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
      description:
        "Describe a TV channel in a sentence. Loomarr builds it, plays it, and keeps it running.",
      social: [
        { icon: "github", label: "GitHub", href: "https://github.com/loomarr/loomarr" },
      ],
      customCss: ["./src/styles/custom.css"],
      sidebar: [
        {
          label: "Install",
          items: [
            { label: "Overview", slug: "install" },
            { label: "Docker", slug: "install/docker" },
            { label: "Hardware acceleration", slug: "install/hardware" },
            { label: "Upgrading", slug: "install/upgrading" },
          ],
        },
        // ⚠ `autogenerate` must sit INSIDE `items`, not beside `label`. Starlight v0.39
        // removed the labelled-autogenerate shorthand; the old form is a config error, not a
        // deprecation warning, so it fails the build outright.
        {
          label: "Using Loomarr",
          items: [{ autogenerate: { directory: "help" } }],
        },
        {
          label: "Development",
          items: [{ autogenerate: { directory: "dev" } }],
        },
        {
          label: "Reference",
          items: [
            { label: "Configuration", slug: "configuration" },
            { label: "Live TV integration", slug: "integrations/media-server-livetv" },
          ],
        },
        {
          label: "Design",
          collapsed: true,
          items: [
            { label: "Design doc (source of truth)", slug: "design" },
            { label: "Programming heuristics", slug: "programming-design" },
            { label: "Configuration subsystem", slug: "config-design" },
            { label: "Frontend design system", slug: "frontend-design" },
          ],
        },
      ],
    }),
  ],
});
