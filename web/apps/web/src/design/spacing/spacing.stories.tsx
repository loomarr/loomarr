import type { Meta, StoryObj } from "@storybook/react-vite";
import { Spacing } from "./spacing";

// The geometry half of the system (§2.3, §2.4): the 4px grid, radii, table density and the one
// motion curve — rendered at their real values so an off-grid number looks off-grid.
const meta = {
  title: "Design/Spacing",
  component: Spacing,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Spacing>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

export default meta;
export { Default };
