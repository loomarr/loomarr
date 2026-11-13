import { Notice } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-native";

const meta = {
  title: "Loomarr Components/Notice",
  component: Notice,
  args: {
    body: "Showing what Loomarr had a moment ago.",
    density: "touch",
    title: "Couldn't refresh the Guide",
  },
} satisfies Meta<typeof Notice>;

type Story = StoryObj<typeof meta>;

const WarnTouch: Story = {
  args: { action: { label: "Try again", onPress: () => undefined }, tone: "warn" },
};
const NeutralTouch: Story = { args: { tone: "neutral" } };
const WarnTv: Story = {
  args: { action: { label: "Try again", onPress: () => undefined }, density: "tv", tone: "warn" },
};
const LightWarn: Story = {
  args: { action: { label: "Try again", onPress: () => undefined }, tone: "warn" },
  globals: { theme: "light" },
};

export default meta;
export { LightWarn, NeutralTouch, WarnTouch, WarnTv };
