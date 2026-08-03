import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { TINY_MP4 } from "@/test/video-fixture";
import { VideoPlayer } from "./video-player";

// The app's video surface, with Loomarr's controls rather than the browser's (V39).
//
// ⚠ **Custom controls are a maintainer decision (2026-08-03), and they cost something.** Native
// `<video controls>` is keyboard-correct and screen-reader-correct for free; hand-built chrome
// makes both this component's job. What it buys is a player that reads as part of the app, which
// matters wherever watching something is part of a decision rather than the point of the page.
//
// ⚠ **Knows nothing about clips.** It takes a `src` and a `title`, both plain strings — the filler
// catalog wraps it in a dialog, and the channel-watch surface the mock sketches would hand it a
// live stream. Putting `ClipDTO` in here would make "core primitive" a lie the first time
// something that is not a clip needed a player.
//
// The scrubber is Radix Slider (§14), not a hand-rolled `role="slider"`: a seek bar IS a slider,
// and the WAI-ARIA keyboard contract is the kind of thing that rots silently when hand-written.
const meta = {
  title: "UI/VideoPlayer",
  component: VideoPlayer,
  decorators: [widthFrame(560)],
} satisfies Meta<typeof VideoPlayer>;

type Story = StoryObj<typeof meta>;

// The default: paused, titled, waiting to be played.
//
// ⚠ No `autoPlay` in any story. A gallery of self-starting videos is hostile, and — more
// practically — an autoplaying player has a moving playhead, so its snapshot would differ on
// every visual run.
const Default: Story = {
  args: { src: TINY_MP4, title: "CTV-LifewithBonnie.mkv — CTV Life with Bonnie" },
};

// Untitled: a player embedded under a heading that already names the thing does not need to
// repeat it, so the whole top overlay disappears rather than rendering an empty scrim.
const NoTitle: Story = {
  args: { src: TINY_MP4 },
};

// A long name truncates rather than wrapping — the title sits on a scrim over arbitrary video, and
// a second line would cover more of the frame than it is worth.
const LongTitle: Story = {
  args: {
    src: TINY_MP4,
    title: "CLE-B01_161770-162673 — Cleveland local broadcast reel, tape 1, side B, segment 4 of 9 (1987)",
  },
};

export default meta;
export { Default, LongTitle, NoTitle };
