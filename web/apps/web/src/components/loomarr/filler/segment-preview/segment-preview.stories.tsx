import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { TINY_MP4 } from "@/test/video-fixture";
import { SegmentPreview } from "./segment-preview";

// One proposed cut, previewed in place (§10 V54). The tile's geometry is the v2 mock's
// (`:2208-2211`): 84×47, striped, a centred ▶ and a duration badge.
//
// `widthFrame` because the expanded panel is `w-full max-w-md` — in the centred canvas it would
// collapse, and these snapshots exist to show it at its real width.
//
// ⚠ **No `autoPlay` in any story.** A moving playhead means a different frame on every visual run,
// and the suite's tolerance is 0.001. The runtime caller passes it, because there the click is the
// gesture browsers require.
const meta = {
  title: "Filler/SegmentPreview",
  component: SegmentPreview,
  decorators: [widthFrame(560)],
} satisfies Meta<typeof SegmentPreview>;

type Story = StoryObj<typeof meta>;

// The resting state, and the one an operator sees 52 of.
const Collapsed: Story = {
  args: {
    clipHash: "a3f9",
    startMs: 257_000,
    endMs: 287_000,
    position: 0,
    labelledBy: "seg-num-0",
    open: false,
    onOpenChange: () => {},
  },
  decorators: [
    (Story) => (
      <div>
        {/* The visible marker the tile borrows for its accessible name. */}
        <span id="seg-num-0" className="sr-only">
          #1
        </span>
        <Story />
      </div>
    ),
  ],
};

// ⚠ `clipHash` is an inline data: URI, not a real hash, and it lives HERE rather than in the
// shared fixtures. `clipAssetURL` passes a `data:` value through untouched, which is what lets
// storybook-static render this offline and deterministically — the trick `clip-player.stories`
// documents at length. A fixture hash would 404 in the static build.
//
// The span matches the fixture's own length so the window does not over-run it.
const Expanded: Story = {
  args: {
    ...Collapsed.args,
    clipHash: TINY_MP4,
    startMs: 0,
    endMs: 2_000,
    open: true,
  },
  decorators: Collapsed.decorators,
};

export default meta;
export { Collapsed, Expanded };
