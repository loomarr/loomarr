import { Stack, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// The alpha washes (#2147): a surface or a brand hue at a fixed alpha, the same in both themes. Set
// as a `backgroundColor` on any Stack or Surface (`backgroundColor="washElevated40"`). Each is drawn
// over the canvas here, with the line of text that sits on it.
const washes = [
  "washCanvas60",
  "washElevated20",
  "washElevated30",
  "washElevated35",
  "washElevated40",
  "washElevated50",
  "washRaised80",
  "washOnair5",
  "washOnair10",
  "washSignal10",
  "washSignal28",
  "washSignal90",
  "washTune5",
] as const;

const Washes = () => (
  <Stack gap="2" padding={24} style={{ maxWidth: 560 }}>
    {washes.map((wash) => (
      <Stack backgroundColor={wash} borderRadius={8} key={wash} padding={12}>
        <Text textRole="label">{wash}</Text>
      </Stack>
    ))}
  </Stack>
);

const meta = {
  title: "Primitives/Alpha washes",
  component: Washes,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Washes>;

type Story = StoryObj<typeof meta>;

const Dark: Story = {};
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Dark, Light };
