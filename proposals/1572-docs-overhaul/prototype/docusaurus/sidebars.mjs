export default {
  docs: [
    "get-started",
    {
      type: "category", label: "Guides", collapsed: false,
      items: [
        "guides/add-a-channel", "guides/tv-app", "guides/hardware-encoding", "guides/filler",
        "guides/watermarks", "guides/4k-hdr", "guides/backups", "guides/upgrade",
        "guides/troubleshooting",
      ],
    },
    {
      type: "category", label: "Reference", collapsed: false,
      items: ["reference/settings", "reference/api", "reference/cli", "reference/hardware"],
    },
    {
      type: "category", label: "Explanation", collapsed: false,
      items: ["explanation/how-loomarr-works", "explanation/playout", "explanation/curation", "explanation/privacy"],
    },
    {
      type: "category", label: "Contributing", collapsed: true,
      items: ["contributing/index", "contributing/ci", "contributing/releasing"],
    },
  ],
};
