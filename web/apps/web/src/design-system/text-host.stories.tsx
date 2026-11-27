import { Stack, Surface, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// Text with an `as` host — the text-style props a caller reaches for (alignment, truncation, line
// clamp, tabular numerals, underline, breaking a long word) drawn on the element it renders, from
// Tamagui's own prop names. Each row names the prop it shows.
const sentence =
  "A channel is a wall clock, not a playlist that starts when someone watches. Tune in forty minutes into an hour-long film and you land forty minutes in.";

const Row = ({ children, name }: { children: React.ReactNode; name: string }) => (
  <Stack gap="1">
    <Text textRole="metadata" tone="secondary">
      {name}
    </Text>
    {children}
  </Stack>
);

const Workshop = ({ width = 420 }: { width?: number }) => (
  <Surface gap="control" padding="section" style={{ maxWidth: width }}>
    <Row name='textAlign="center"'>
      <Text as="p" textAlign="center" textRole="compact">
        Nothing is scheduled yet
      </Text>
    </Row>
    <Row name="numberOfLines={1}">
      <Text as="p" numberOfLines={1} textRole="compact">
        {sentence}
      </Text>
    </Row>
    <Row name="numberOfLines={2}">
      <Text as="p" numberOfLines={2} textRole="compact">
        {sentence}
      </Text>
    </Row>
    <Row name='fontVariant={["tabular-nums"]}'>
      <Stack gap="none">
        <Text as="p" fontVariant={["tabular-nums"]} textRole="compact">
          1,111.11
        </Text>
        <Text as="p" fontVariant={["tabular-nums"]} textRole="compact">
          8,888.88
        </Text>
      </Stack>
    </Row>
    <Row name='textDecorationLine="underline"'>
      <Text as="p" textDecorationLine="underline" textRole="compact">
        Underlined copy
      </Text>
    </Row>
    <Row name='overflowWrap="anywhere"'>
      <Text as="p" overflowWrap="anywhere" textRole="mono">
        /media/library/television/a-very-long-show-name-with-no-spaces/season-01/episode-0001-the-pilot.mkv
      </Text>
    </Row>
  </Surface>
);

const meta = {
  title: "Loomarr Components/Text host props",
  component: Workshop,
} satisfies Meta<typeof Workshop>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Light: Story = { globals: { theme: "light" } };
const Narrow: Story = { args: { width: 260 } };

export default meta;
export { Default, Light, Narrow };
