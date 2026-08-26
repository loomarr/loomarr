import { StatePanel } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";

const meta = {
  title: "Loomarr Components/Feedback and Recovery",
  component: StatePanel,
  args: {
    density: "touch",
    description: "Choose another time or create a channel to put something on air.",
    kind: "empty",
    title: "Nothing scheduled yet",
  },
} satisfies Meta<typeof StatePanel>;

type Story = StoryObj<typeof meta>;
const EmptyTouch: Story = { args: { action: { label: "Open Guide", onPress: () => undefined } } };
const OfflineTouch: Story = {
  args: {
    action: { label: "Reconnect", onPress: () => undefined },
    kind: "offline",
    title: "Loomarr is offline",
  },
};
const LoadingTv: Story = { args: { density: "tv", kind: "loading", title: "Loading channels" } };
const PermissionTv: Story = {
  args: {
    action: { label: "Pair again", onPress: () => undefined },
    density: "tv",
    kind: "permission",
    title: "This TV was disconnected",
  },
};
const LightEmpty: Story = {
  args: { action: { label: "Open Guide", onPress: () => undefined } },
  globals: { theme: "light" },
};

export default meta;
export { EmptyTouch, LightEmpty, LoadingTv, OfflineTouch, PermissionTv };
