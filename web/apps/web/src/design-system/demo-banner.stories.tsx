import { DemoBanner } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// The demo tour banner (#2268): on a demo site, the top of every page says so and offers the next
// thing to try. Each step completes when its page is visited; the last state says the tour is done.
// The links are plain anchors here; the app passes router links.

const meta = {
  title: "Loomarr Components/Demo Banner",
  component: DemoBanner,
  decorators: [
    (Story) => (
      <div style={{ boxSizing: "border-box", maxWidth: 1040 }}>
        <Story />
      </div>
    ),
  ],
  args: {
    step: 1,
    stepCount: 3,
    next: { label: "Watch a channel live", link: <a href="/watch/ch-1">Watch a channel live</a> },
    onDismiss: () => undefined,
  },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof DemoBanner>;

type Story = StoryObj<typeof meta>;

const FirstStep: Story = {};
const SecondStep: Story = {
  args: { step: 2, next: { label: "Browse the Guide", link: <a href="/guide">Browse the Guide</a> } },
};
const LastStep: Story = {
  args: {
    step: 3,
    next: { label: "Look inside a channel", link: <a href="/channels/ch-1">Look inside a channel</a> },
  },
};
const Done: Story = { args: { step: 3, next: undefined } };
// A phone-width page: the next step wraps under the message rather than squeezing it.
const Narrow: Story = {
  decorators: [
    (Story) => (
      <div style={{ boxSizing: "border-box", maxWidth: 390 }}>
        <Story />
      </div>
    ),
  ],
};
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Done, FirstStep, LastStep, Light, Narrow, SecondStep };
