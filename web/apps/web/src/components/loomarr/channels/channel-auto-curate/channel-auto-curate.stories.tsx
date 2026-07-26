import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { TooltipProvider } from "@/components/ui";
import { widthFrame } from "@/test/story-utils";
import { ChannelAutoCurate } from "./channel-auto-curate";

const noop = () => {};

// The opt-in's help is an (i) FieldHelp tooltip, which needs a TooltipProvider ancestor
// (mounted at the app root in the real app; supplied here for isolation).
const withTooltip: Decorator = (Story) => (
  <TooltipProvider>
    <Story />
  </TooltipProvider>
);

// The per-channel auto-curate opt-in (programming-design §8.2). The states below are the three
// the component actually distinguishes — and "opted in, inheriting" vs "opted in, overridden"
// look different for a reason: 0 is the inherit sentinel, so an inherited threshold must render
// blank ("Default"), never as a literal 0.
const meta = {
  title: "Channels/ChannelAutoCurate",
  component: ChannelAutoCurate,
  args: { onChange: noop },
  decorators: [withTooltip, widthFrame(480)],
} satisfies Meta<typeof ChannelAutoCurate>;

type Story = StoryObj<typeof meta>;

// Off: no `autoCurate` key at all. The overrides stay hidden — showing disabled boxes for a
// feature that is off reads as broken rather than inapplicable.
const Off: Story = { args: { policy: {} } };

// On, inheriting both global thresholds. THE structural case: a zero-value object means
// "opted in", because the opt-in is the object's presence.
const OptedIn: Story = { args: { policy: { autoCurate: {} } } };

// On, with both thresholds overridden — stricter than the fleet default on quality, and capped.
const Overridden: Story = {
  args: { policy: { autoCurate: { minScorePct: 75, maxTitles: 40 } } },
};

// A hand-made channel: §8.2's job re-runs a channel's stored intent, so with no IntentRef there
// is nothing to re-evaluate. Disabled with the reason stated, rather than accepting a setting
// that would never fire.
const HandMade: Story = { args: { policy: {}, intentBacked: false } };

export default meta;
export { HandMade, Off, OptedIn, Overridden };
