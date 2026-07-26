import type { Meta, StoryObj } from "@storybook/react-vite";
import { Typography } from "./typography";

// The type system (§2.2, §5.1a): the two families, the rule that decides between them, and the
// scale — rendered at their real values so an off-scale size is visibly off-scale.
const meta = {
  title: "Design/Typography",
  component: Typography,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Typography>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

export default meta;
export { Default };
