import type { Meta, StoryObj } from "@storybook/react-vite";
import { Tokens } from "./tokens";

// The complete generated token set as a reference table (§2.5). The other Design pages explain
// the system; this one is what you scan for the exact name of a thing.
const meta = {
  title: "Design/Tokens",
  component: Tokens,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Tokens>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

export default meta;
export { Default };
