import { ChannelGrowthSection } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// What a channel takes in by itself (#4333): on the channel's Info page, for admins.

const meta = {
  title: "Loomarr Components/Channel Growth Section",
  component: ChannelGrowthSection,
  decorators: [
    (Story) => (
      <div style={{ boxSizing: "border-box", maxWidth: 720, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
  args: {
    joined: [
      { batch: "growth-2026-11-18", key: "series:tvdb:90000002", name: "Night Desk" },
      { batch: "growth-2026-11-18", key: "series:tvdb:90000001", name: "Harbor Lights" },
      { batch: "growth-2026-11-16", key: "movie:tmdb:90000003", name: "The Long Weekend" },
    ],
    keep: false,
    lineWords: "series from 1986 to 1995, rated up to TV-PG, comedy and family",
    off: [
      { key: "series:tvdb:90000004", name: "Quiet Harbor", reason: "Too grown-up for this channel" },
      { key: "series:tvdb:90000005", name: "Paper Moon Diner", reason: "" },
    ],
    onAddBack: () => undefined,
    onKeepChange: () => undefined,
    onTakeOff: () => undefined,
    short: { episodes: 120, films: 0, shows: 2 },
    still: "",
    target: { episodes: 400, films: 0, shows: 8 },
    waiting: 3,
  },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ChannelGrowthSection>;

type Story = StoryObj<typeof meta>;

const Growing: Story = {};
const KeptAsItIs: Story = { args: { keep: true, still: "kept" } };
const EnoughAlready: Story = {
  args: { short: { episodes: 0, films: 0, shows: 0 }, waiting: 0, off: [] },
};
const MadeByHand: Story = { args: { joined: [], lineWords: "", off: [], still: "by_hand" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { EnoughAlready, Growing, KeptAsItIs, Light, MadeByHand };
