// PROTOTYPE for #1572: Docusaurus 3 reading the same sample tree in place, same tokens.
import githubAlerts from "remark-github-admonitions-to-directives";

export default {
  title: "Loomarr",
  tagline: "Describe a TV channel in a sentence.",
  url: "https://mantonx.github.io",
  baseUrl: "/loomarr/",
  onBrokenLinks: "warn",
  // Plain CommonMark for .md, so the in-app help and GitHub read the same files unchanged.
  markdown: { format: "detect" },
  presets: [
    [
      "classic",
      {
        docs: {
          path: "../proto-docs",
          include: ["get-started.md", "guides/**/*.md", "reference/**/*.md", "explanation/**/*.md", "contributing/**/*.md"],
          routeBasePath: "/",
          sidebarPath: "./sidebars.mjs",
          beforeDefaultRemarkPlugins: [githubAlerts],
          editUrl: "https://github.com/loomarr/loomarr/edit/main/docs/",
        },
        blog: false,
        theme: { customCss: ["./node_modules/@fontsource-variable/geist/index.css", "./src/css/custom.css"] },
      },
    ],
  ],
  themeConfig: {
    colorMode: { respectPrefersColorScheme: true },
    navbar: {
      title: "Loomarr",
      items: [{ href: "https://github.com/loomarr/loomarr", label: "GitHub", position: "right" }],
    },
  },
};
