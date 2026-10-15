import { ClientShell } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";

const meta = {
  title: "Loomarr Components/Client Shell",
  component: ClientShell,
  args: {
    active: "guide",
    onNavigate: () => {},
  },
} satisfies Meta<typeof ClientShell>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Touch };
