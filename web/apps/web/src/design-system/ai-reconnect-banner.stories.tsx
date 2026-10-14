import { AiReconnectBanner } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// The manual builder's resume banner (#1817, approved mock #1872 Alt A): AI came back while an
// admin was picking titles by hand. Both actions keep the picks; only "Try AI suggestions" asks AI
// for more.

const meta = {
  title: "Loomarr Components/AI Reconnect Banner",
  component: AiReconnectBanner,
  decorators: [
    (Story) => (
      <div style={{ boxSizing: "border-box", maxWidth: 920, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
  args: {
    pickCount: 3,
    onTryAi: () => undefined,
    onKeepManual: () => undefined,
  },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof AiReconnectBanner>;

type Story = StoryObj<typeof meta>;

const WithPicks: Story = {};
const OnePick: Story = { args: { pickCount: 1 } };
// Nothing picked yet: "Try AI suggestions" goes back to the describe form with the description.
const NothingPicked: Story = { args: { pickCount: 0 } };
// Picks without a channel name: the hand-off can't record them yet, and says why.
const NeedsName: Story = { args: { tryAiBlockedReason: "Name this channel to try AI suggestions." } };
const HandingOver: Story = { args: { busy: true } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { HandingOver, Light, NeedsName, NothingPicked, OnePick, WithPicks };
