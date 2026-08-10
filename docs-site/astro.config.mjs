// @ts-check
import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";

// Published to GitHub Pages at https://mantonx.github.io/loomarr/. `base` must match the repo
// name or every internal link 404s on the deployed site while working perfectly in dev.
export default defineConfig({
  site: "https://mantonx.github.io",
  base: "/loomarr",
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
        { icon: "github", label: "GitHub", href: "https://github.com/mantonx/loomarr" },
      ],
      // Mermaid, client-side. Starlight has no native support, and rehype-mermaid renders
      // through a headless browser — a Playwright download in the docs build is not worth a
      // diagram (design §14). This renders the same fenced blocks GitHub already renders, so
      // one source serves both.
      customCss: ["./src/styles/custom.css"],
      components: {
        Head: "./src/components/Head.astro",
      },
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
        {
          label: "Using Loomarr",
          autogenerate: { directory: "help" },
        },
        {
          label: "Development",
          autogenerate: { directory: "dev" },
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
