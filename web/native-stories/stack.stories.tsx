import { Screen, Stack, Surface, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-native";

const Tile = ({ children }: { children: string }) => (
  <Surface padding="control">
    <Text textRole="label">{children}</Text>
  </Surface>
);

const NativeStackWorkshop = ({ density = "touch" }: { density?: "touch" | "tv" }) => (
  <Screen density={density}>
    <Stack gap="section">
      <Text density={density} textRole="title">
        Stack
      </Text>
      <Stack gap="control">
        <Tile>One</Tile>
        <Tile>Two</Tile>
      </Stack>
      <Stack align="center" direction="row" gap="inline" justify="between" wrap>
        <Tile>Start</Tile>
        <Tile>Middle</Tile>
        <Tile>End</Tile>
      </Stack>
    </Stack>
  </Screen>
);

const meta = {
  title: "Loomarr Foundations/Stack",
  component: NativeStackWorkshop,
  args: { density: "touch" },
} satisfies Meta<typeof NativeStackWorkshop>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };

export default meta;
export { Touch, Tv };
