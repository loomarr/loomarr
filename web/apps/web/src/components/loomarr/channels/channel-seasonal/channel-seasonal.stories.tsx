import type { Vocabulary } from "@loomarr/api";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { TooltipProvider } from "@/components/ui";
import { widthFrame } from "@/test/story-utils";
import { ChannelSeasonal } from "./channel-seasonal";

const noop = () => {};

const withTooltip: Decorator = (Story) => (
  <TooltipProvider>
    <Story />
  </TooltipProvider>
);

// Shaped like the BE's: holidays ride the `when` list as `holiday:<id>` tokens, alongside the
// non-holiday WHEN tokens the picker filters out.
const vocabulary = {
  when: [
    { token: "primetime", label: "Primetime (20–23)", shortLabel: "Primetime", priority: 1, predicate: {} },
    { token: "holiday:christmas", label: "Christmas", shortLabel: "Christmas", priority: 2, predicate: {} },
    { token: "holiday:halloween", label: "Halloween", shortLabel: "Halloween", priority: 2, predicate: {} },
    {
      token: "holiday:thanksgiving",
      label: "Thanksgiving",
      shortLabel: "Thanksgiving",
      priority: 2,
      predicate: {},
    },
    { token: "holiday:newyear", label: "New Year", shortLabel: "New Year", priority: 2, predicate: {} },
  ],
  what: [],
  how: [],
} as unknown as Vocabulary;

// Holiday-aware programming (programming-design §6). The states below are the ones that render
// differently: mode drives whether the holiday list and the off-season fallback appear at all.
const meta = {
  title: "Channels/ChannelSeasonal",
  component: ChannelSeasonal,
  args: { onChange: noop, vocabulary },
  decorators: [withTooltip, widthFrame(480)],
} satisfies Meta<typeof ChannelSeasonal>;

type Story = StoryObj<typeof meta>;

// Unset: the "" sentinel resolves to auto. No holidays picked means ALL of them.
const Default: Story = { args: { policy: {} } };

// Holidays ignored entirely — the picker is hidden because nothing would read it.
const Off: Story = { args: { policy: { seasonal: { mode: "off" } } } };

// A specific subset chosen, in auto mode.
const Picked: Story = {
  args: { policy: { seasonal: { mode: "auto", holidays: ["halloween", "christmas"] } } },
};

// Exclusive is the only mode that consults the off-season fallback, so it is the only one
// that renders it (schedule/seasonal.go:154).
const HolidayChannel: Story = {
  args: { policy: { seasonal: { mode: "exclusive", holidays: ["christmas"], offSeason: "dark" } } },
};

// Built-in rotation in October with the backend's count for the open holiday: the consequence
// line under Holiday strength (§6 "Holiday episodes").
const HalloweenOn: Story = {
  args: {
    policy: {},
    holidayEpisodes: [
      { holiday: "halloween", episodes: 9, perDay: 3, everyDays: 3, active: true },
      { holiday: "thanksgiving", episodes: 0, perDay: 0, everyDays: 0, active: false },
    ],
  },
};

// Heavy, spread across the day: the evenings opt-out reads back unchecked with its own hint, and
// a small pool caps Heavy (nine episodes give four a day).
const HeavyAnytime: Story = {
  args: {
    policy: { seasonal: { strength: "heavy", placement: "anytime" } },
    holidayEpisodes: [{ holiday: "halloween", episodes: 9, perDay: 4, everyDays: 2, active: true }],
  },
};

export default meta;
export { Default, HalloweenOn, HeavyAnytime, HolidayChannel, Off, Picked };
