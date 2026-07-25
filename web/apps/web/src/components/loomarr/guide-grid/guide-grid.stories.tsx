import { guideChannels, guideFrom, guideNow, guideTo } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { GuideGrid } from "./guide-grid";

// The cross-channel schedule (§12): every channel a row, time the shared horizontal axis,
// each block's width IS its duration. The fixture window is FIXED epoch ms — the grid
// positions everything against `fromMs`, so a clock-derived origin would move every block
// and the visual suite would diff on nothing but the time of day.
const meta = {
  title: "Loomarr/GuideGrid",
  component: GuideGrid,
  args: { channels: guideChannels, fromMs: guideFrom, toMs: guideTo, nowMs: guideNow },
  decorators: [widthFrame(880)],
} satisfies Meta<typeof GuideGrid>;

type Story = StoryObj<typeof meta>;

// The everyday view: programmes, commercial breaks, a pending acquisition and a flex gap,
// all four visually distinct — the reason the API carries a `kind` discriminator rather
// than the boolean it replaced.
const Default: Story = {};

// Zoomed in. Zoom is ONE number (px per minute); doubling it magnifies the schedule rather
// than redrawing it, so short blocks become legible without the columns shifting.
const ZoomedIn: Story = { args: { pxPerMinute: 8 } };

// Zoomed out to a wider window — where the minimum-width floor matters, since a 4-minute
// commercial break at this scale is only a few pixels wide but must still be visible.
const ZoomedOut: Story = { args: { pxPerMinute: 1.5 } };

// Before the window's "now": no line is drawn, because pinning it to an edge would claim
// the current instant is on screen when it is not.
const NowOutsideWindow: Story = { args: { nowMs: guideFrom - 3_600_000 } };

// A channel that has nothing scheduled still gets a row — dropping it would read as the
// channel having been deleted rather than as an empty evening.
const EmptyChannel: Story = {
  args: {
    channels: [...guideChannels, { channelId: "ch-quiet", name: "Late Night", number: 9, airings: [] }],
  },
};

export default meta;
export { Default, EmptyChannel, NowOutsideWindow, ZoomedIn, ZoomedOut };
