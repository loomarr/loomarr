import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { SegmentFilmstrip } from "./segment-filmstrip";

// The reel at a glance (§10, the v2 mock's `rl.strip`). Every block is one detected clip, sized
// in PROPORTION to its duration — which is what makes an over-split reel legible without reading
// a single row.
//
// `widthFrame` because the strip is `w-full`: in the centred story canvas it would otherwise
// collapse, and these snapshots exist to show the block WIDTHS.
const meta = {
  title: "Filler/SegmentFilmstrip",
  component: SegmentFilmstrip,
  decorators: [widthFrame(640)],
} satisfies Meta<typeof SegmentFilmstrip>;

type Story = StoryObj<typeof meta>;

// A well-split reel: a dozen adverts of broadly similar length, which is what a good detection
// run looks like.
const EvenlySplit: Story = {
  args: {
    segments: Array.from({ length: 12 }, (_, i) => ({
      key: `s${i}`,
      startMs: i * 30_000,
      endMs: (i + 1) * 30_000,
      name: `Advert ${i + 1}`,
    })),
  },
};

// ⚠ The case the strip is FOR. One 14-minute block beside eleven short ones says "detection
// missed the boundaries in here" at a glance — the judgement that would otherwise mean reading
// twelve rows of timecodes.
const OneHugeBlock: Story = {
  args: {
    segments: [
      ...Array.from({ length: 11 }, (_, i) => ({
        key: `s${i}`,
        startMs: i * 20_000,
        endMs: (i + 1) * 20_000,
        name: `Advert ${i + 1}`,
      })),
      { key: "blob", startMs: 220_000, endMs: 1_060_000, name: "Unsplit remainder", unsplittable: true },
    ],
  },
};

// A block focused from the strip, tinted so it and the row below agree about what is selected.
const Focused: Story = {
  args: {
    segments: [
      { key: "a", startMs: 0, endMs: 30_000, name: "Frosted Flakes" },
      { key: "b", startMs: 30_000, endMs: 45_000, name: "Station ident" },
      { key: "c", startMs: 45_000, endMs: 105_000, name: "Toy ad" },
    ],
    activeKey: "b",
  },
};

// ⚠ A very short segment beside a very long one. The floor keeps the sting clickable rather than
// letting it compute to a sub-pixel sliver nobody can hit.
const TinySegment: Story = {
  args: {
    segments: [
      { key: "sting", startMs: 0, endMs: 500, name: "Sting" },
      { key: "feature", startMs: 500, endMs: 600_000, name: "Long block" },
    ],
  },
};

export default meta;
export { EvenlySplit, Focused, OneHugeBlock, TinySegment };
