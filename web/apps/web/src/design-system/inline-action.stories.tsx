import { InlineAction, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// InlineAction — a button written into a line of prose. It wraps with the words around it and
// takes their type; only the underline, the primary colour and the focus outline are its own.
const Sentence = ({ disabled = false, width = 520 }: { disabled?: boolean; width?: number }) => (
  <div style={{ maxWidth: width, padding: 24 }}>
    <Text as="p" textRole="hint" tone="secondary">
      Following this installation (US · Springfield). A channel keeps this place until you pick another.{" "}
      <InlineAction disabled={disabled} onPress={() => undefined}>
        Use another place
      </InlineAction>
    </Text>
  </div>
);

const meta = {
  title: "Loomarr Components/InlineAction",
  component: Sentence,
} satisfies Meta<typeof Sentence>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Light: Story = { globals: { theme: "light" } };
// Narrow: the label breaks across lines with the sentence rather than jumping to its own line.
const Wrapping: Story = { args: { width: 260 } };
const Disabled: Story = { args: { disabled: true } };

export default meta;
export { Default, Disabled, Light, Wrapping };
