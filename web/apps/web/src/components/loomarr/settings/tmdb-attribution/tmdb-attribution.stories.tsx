import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { TmdbAttribution } from "./tmdb-attribution";

const meta = {
  title: "Settings/TmdbAttribution",
  component: TmdbAttribution,
  decorators: [widthFrame(760)],
} satisfies Meta<typeof TmdbAttribution>;

type Story = StoryObj<typeof meta>;

// What ships today: the notice on its own. The logo is TMDB's trademark and this repository
// carries no copy of it, so the default state must read correctly without one.
const Default: Story = {};

// With a logo supplied by the build. The mark is a local placeholder rectangle rather than TMDB's
// actual logo — a story is not the place to vendor someone's trademark, and what this story exists
// to show is the LAYOUT: that the mark renders small and muted beside the notice, satisfying the
// "less prominent than our own branding" half of the requirement.
//
// ⚠ An inline data: URI, never a remote URL — a story that fetches over the network races its own
// visual snapshot (and would be blocked offline in storybook-static).
const WithLogo: Story = {
  args: {
    logo: (
      <img
        src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 16'%3E%3Crect width='64' height='16' rx='2' fill='%2301b4e4'/%3E%3C/svg%3E"
        alt="TMDB"
      />
    ),
  },
};

export default meta;
export { Default, WithLogo };
