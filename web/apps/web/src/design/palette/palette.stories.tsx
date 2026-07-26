import type { Meta, StoryObj } from "@storybook/react-vite";
import { Palette } from "./palette";

// The palette, rendered from the generated tokens and measured live (§2.1, §5.1a). This is a
// DESIGN page rather than a component: it has one state, and its job is to make the contrast
// rules visible to anyone opening the workshop.
const meta = {
  title: "Design/Palette",
  component: Palette,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Palette>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

export default meta;
export { Default };
