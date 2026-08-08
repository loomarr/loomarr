import type { Meta, StoryObj } from "@storybook/react-vite";
import { TunerLoader } from "./tuner-loader";

// The "acquiring signal" loader for the Watch player (§9.1). It is absolute inset-0, so the story
// frames it in a relative 16:9 box on black — the player frame it overlays. Under the visual suite's
// reduced-motion pin the snow (TvStatic) renders empty and the bars sit at their LOCKED amber frame
// (steady height + signal-400), so the gallery baseline is the settled "signal acquired" look, not a
// mid-flicker one — deterministic by construction.
const meta = {
  title: "Shell/TunerLoader",
  component: TunerLoader,
  decorators: [
    (Story) => (
      <div className="relative aspect-video w-96 overflow-hidden rounded-xl bg-black">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof TunerLoader>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

// A custom label — the slot the caller can rename (e.g. a different surface's warm-up copy).
const CustomLabel: Story = { args: { label: "ACQUIRING SIGNAL" } };

export default meta;
export { CustomLabel, Default };
