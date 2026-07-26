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

export default meta;
export { Compact, Hero };
