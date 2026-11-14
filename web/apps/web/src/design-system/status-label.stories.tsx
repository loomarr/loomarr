import { Stack, StatusLabel } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// StatusLabel — quiet status: a dot and a sentence-case word, no fill. Given `onPress` the same
// label is a button, for a state that has something to do about it (a setting's "Overriding
// LOOMARR_PORT" hands the key back to the environment). The look is unchanged; the button adds a
// focus stop and the one amber outline.
const meta = {
  title: "Primitives/StatusLabel",
  component: StatusLabel,
  args: { children: "Set via environment", tone: "off" },
} satisfies Meta<typeof StatusLabel>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

const Pressable: Story = {
  args: {
    "aria-label": "Overriding LOOMARR_PORT: hand Port back, your saved value is kept",
    children: "Overriding LOOMARR_PORT",
    onPress: () => undefined,
    tone: "warn",
  },
};

const Tones: Story = {
  render: () => (
    <Stack align="start" gap="3">
      <StatusLabel tone="ok">Enabled</StatusLabel>
      <StatusLabel tone="warn">Overriding LOOMARR_PORT</StatusLabel>
      <StatusLabel onPress={() => undefined} tone="warn">
        Overriding LOOMARR_PORT
      </StatusLabel>
      <StatusLabel tone="off">Set via environment</StatusLabel>
    </Stack>
  ),
};

export default meta;
export { Default, Pressable, Tones };
