import { AboutPage } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// Settings → System → About: Loomarr's lockup and version, the Data sources section with TMDB's
// logo and notice (#2853), and the bug-report facts behind a disclosure. The TMDB mark is the
// vendored public/ file, not a remote image.

const meta = {
  title: "Loomarr Components/About",
  component: AboutPage,
  args: {
    version: "v0.9.3",
    tmdbLogo: <img alt="TMDB" src="/tmdb.svg" style={{ height: 16, width: "auto" }} />,
    details: [
      { label: "Version", value: "v0.9.3" },
      { label: "Commit", value: "9871fef2ac10" },
      { label: "Built", value: "2026-07-22T18:04:00Z" },
      { label: "Go runtime", value: "go1.23.4 · linux/amd64" },
      { label: "Uptime", value: "6d 4h 12m" },
      { label: "Database", value: "sqlite · schema 20" },
      { label: "Status", value: "Ready" },
    ],
  },
} satisfies Meta<typeof AboutPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

// The demo site: its openly licensed films, cartoons and ads, and where their sources are listed.
export const DemoSite: Story = { args: { demo: true } };

// A source build: no linker stamps, so only the version and status rows exist.
export const SourceBuild: Story = {
  args: {
    version: "dev",
    details: [
      { label: "Version", value: "dev" },
      { label: "Status", value: "Ready" },
    ],
  },
};
