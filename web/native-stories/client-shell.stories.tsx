import { ClientShell } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";

const meta = {
  title: "Loomarr/Client Shell",
  component: ClientShell,
  args: {
    active: "guide",
    density: "touch",
    onDisconnect: () => {},
    onNavigate: () => {},
    serverName: "loomarr.media",
  },
} satisfies Meta<typeof ClientShell>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const TvGuide: Story = { args: { density: "tv" } };
const TvWatching: Story = { args: { active: "watching", density: "tv" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Touch, TvGuide, TvWatching };
