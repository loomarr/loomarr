import { ChannelUnairable } from "@loomarr/ui/channels";
import type { Meta, StoryObj } from "@storybook/react-vite";

// The lineup titles a channel cannot air, and why (#4140): on the channel's Info page, for admins.

const meta = {
  title: "Loomarr Components/Channel Unairable",
  component: ChannelUnairable,
  decorators: [
    (Story) => (
      <div style={{ boxSizing: "border-box", maxWidth: 720, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
  args: {
    titles: [
      { key: "series:tvdb:90000012", reason: "no_episodes_in_seasons", title: "Harbor Lights" },
      { key: "movie:tmdb:90000002", reason: "not_in_library", title: "The Long Weekend" },
    ],
  },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ChannelUnairable>;

type Story = StoryObj<typeof meta>;

const TwoTitles: Story = {};
const OneTitle: Story = {
  args: { titles: [{ key: "series:tvdb:90000014", reason: "over_ceiling", title: "Night Desk" }] },
};
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, OneTitle, TwoTitles };
