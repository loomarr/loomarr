import { StatePanel } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

const meta = {
  title: "Loomarr Components/Feedback and Recovery",
  component: StatePanel,
  decorators: [
    (Story) => (
      <div style={{ boxSizing: "border-box", minHeight: "100vh", padding: 48 }}>
        <Story />
      </div>
    ),
  ],
  args: {
    density: "pointer",
    kind: "empty",
    title: "Nothing scheduled yet",
  },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof StatePanel>;

type Story = StoryObj<typeof meta>;

const Empty: Story = {
  args: {
    action: { label: "Open Guide", onPress: () => undefined },
    description: "Choose another time or create a channel to put something on air.",
  },
};
const Loading: Story = { args: { kind: "loading", title: "Loading channels" } };
const ErrorWithRecovery: Story = {
  args: {
    action: { label: "Try again", onPress: () => undefined },
    description: "Loomarr kept the last programme on screen while it reconnects.",
    kind: "error",
    title: "Couldn’t refresh the guide",
  },
};
const OfflineTouch: Story = {
  args: {
    action: { label: "Reconnect", onPress: () => undefined },
    density: "touch",
    description: "Check this device’s connection. Your paired credential is still protected.",
    kind: "offline",
    title: "Loomarr is offline",
  },
};
const PermissionTv: Story = {
  args: {
    action: { label: "Pair again", onPress: () => undefined },
    density: "tv",
    description: "This device no longer has permission to use this Loomarr server.",
    kind: "permission",
    title: "This TV was disconnected",
  },
};
const Light: Story = {
  ...Empty,
  globals: { theme: "light" },
};

export default meta;
export { Empty, ErrorWithRecovery, Light, Loading, OfflineTouch, PermissionTv };
