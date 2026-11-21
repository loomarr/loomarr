import { Card, Stack, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// The tinted frames (#2147): a 40% hairline of a brand hue over a 5% wash of it, beside the plain
// raised card.
const tones = [
  ["caution", "Heads up: the library is nearly full"],
  ["signal", "3 clips selected"],
  ["onair", "That didn't save"],
  ["suggest", "Loomarr has a proposal"],
  ["tune", "Tuned to Channel 4"],
] as const;

const Tones = () => (
  <Stack gap="3" padding={24} style={{ maxWidth: 560 }}>
    <Card gap="1" padding={16}>
      <Text textRole="label">Plain raised card</Text>
      <Text textRole="hint" tone="secondary">
        No tone: the raised fill and the decorative border.
      </Text>
    </Card>
    {tones.map(([tone, line]) => (
      <Card gap="1" key={tone} padding={16} tone={tone}>
        <Text textRole="label">{line}</Text>
        <Text textRole="hint" tone="secondary">
          tone="{tone}"
        </Text>
      </Card>
    ))}
  </Stack>
);

const meta = {
  title: "Primitives/Card",
  component: Tones,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Tones>;

type Story = StoryObj<typeof meta>;

const Dark: Story = {};
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Dark, Light };
