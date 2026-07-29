import type { Meta, StoryObj } from "@storybook/react-vite";
import { ColorBars } from "./color-bars";

// The test-card strip (§1). Both sizes so the gallery pins the segment colors (they ARE
// the accent tokens) and the two footprints the app uses: the login/wizard hero and the
// sidebar lockup.
const meta = {
  title: "Shell/ColorBars",
  component: ColorBars,
} satisfies Meta<typeof ColorBars>;

type Story = StoryObj<typeof meta>;

const Hero: Story = { args: { size: "lg" } };
const Compact: Story = { args: { size: "sm" } };

// The guide's "Dead air" card: segments breathing out of phase, at the block footprint the
// empty state actually uses (the sidebar's thin strip is too small to read a shimmer on).
//
// ⚠ Its BASELINE is a still frame — the visual suite pins `reducedMotion: reduce`, so
// `motion-safe:animate-bar-breathe` compiles away and the bars snapshot at full opacity.
// That the animation exists at all is asserted in tests/visual/motion.spec.ts, which is the
// one suite that runs with motion enabled.
const Breathing: Story = {
  args: { size: "lg", breathe: true, className: "h-16 w-50" },
};

export default meta;
export { Breathing, Compact, Hero };
