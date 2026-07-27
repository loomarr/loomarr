import { guideChannels, guideFrom, guideNow, guideTo } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { GuideGrid } from "./guide-grid";

// The cross-channel schedule (§12): a fixed channel rail, then one flexed time area per row
// where every block's width IS its duration. The fixture window is FIXED epoch ms — every
// position is relative to `fromMs`, so a clock-derived origin would move every block and the
// visual suite would diff on nothing but the time of day.
const meta = {
  title: "Guide/GuideGrid",
  component: GuideGrid,
  args: { channels: guideChannels, fromMs: guideFrom, toMs: guideTo, nowMs: guideNow },
  decorators: [widthFrame(1000)],
} satisfies Meta<typeof GuideGrid>;

type Story = StoryObj<typeof meta>;

// The everyday view: programmes, a break rendering its own clips, a pending placeholder and a
// flex gap — all four visually distinct, which is why the API carries a `kind` discriminator
// rather than the boolean it replaced. The now-line crosses every row, and whatever is airing
// at that instant is highlighted.
const Default: Story = {};

// Zoom magnifies the TIME AXIS, not the chrome. At 2× an hour occupies twice the pixels, so the
// grid overflows its viewport and scrolls horizontally — which is what makes a short commercial
// break resolve into a labelled block instead of an unreadable smear. The rail, row height and
// type are deliberately unchanged at every zoom: scaling them was what made titles illegible.
const ZoomedIn: Story = { args: { zoom: 2 } };

// Below 1 the whole window still fits; the schedule just gets denser, which is the mode for
// scanning a long day rather than reading one.
const ZoomedOut: Story = { args: { zoom: 0.75 } };

// Before the window's start: no line is drawn, because pinning it to an edge would claim the
// current instant is on screen when it is not.
const NowOutsideWindow: Story = { args: { nowMs: guideFrom - 3_600_000 } };

// A wider window switches the ruler to hourly ticks — half-hours across a whole day are noise.
const FullDay: Story = {
  args: { fromMs: guideFrom, toMs: guideFrom + 12 * 60 * 60_000, nowMs: guideFrom + 60 * 60_000 },
};

// A channel with nothing scheduled still gets a row — dropping it would read as the channel
// having been deleted rather than as an empty evening.
const EmptyChannel: Story = {
  args: {
    channels: [
      ...guideChannels,
      { channelId: "ch-quiet", name: "Late Night", number: 9, status: "live", pendingCount: 0, airings: [] },
    ],
  },
};

export default meta;
export { Default, EmptyChannel, FullDay, NowOutsideWindow, ZoomedIn, ZoomedOut };
