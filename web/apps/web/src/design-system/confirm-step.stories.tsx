import { ConfirmStep } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// ConfirmStep — the second press of an irreversible action, the same two-step shape as Delete channel.
const meta = {
  title: "Loomarr Components/ConfirmStep",
  component: ConfirmStep,
  args: {
    confirmLabel: "Remove device",
    message: "Remove Living Room Shield? It signs out now and has to be paired again to watch.",
    onCancel: () => undefined,
    onConfirm: () => undefined,
  },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 480, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof ConfirmStep>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
// The action is already running, so the confirm holds.
const Busy: Story = { args: { disabled: true } };

export default meta;
export { Busy, Default };
