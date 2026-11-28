import { Icon, icons, RingedPressable, Surface, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";

// RingedPressable: a flat face with one ring. At rest, on, drawn shorter than its target, and ringed
// by the keyboard or a pointer.

const meta = {
  title: "Design System/Ringed Pressable",
  component: RingedPressable,
  args: { accessibilityLabel: "Pause", children: null, size: 44, tip: "Pause (Space)" },
  render: (args) => (
    <Surface backgroundColor="surfaceCanvas" borderWidth={0} flexDirection="row" gap={16} padding={48}>
      <RingedPressable {...args}>
        <Icon decorative glyph={icons.pause} size="touch" tone="content" />
      </RingedPressable>
      <RingedPressable accessibilityLabel="Channels" expanded on size={44}>
        <Text textRole="label">Channels</Text>
      </RingedPressable>
      <RingedPressable accessibilityLabel="Go live" faceHeight={28} size={44}>
        <Text textRole="label" tone="live">
          Go live
        </Text>
      </RingedPressable>
    </Surface>
  ),
} satisfies Meta<typeof RingedPressable>;

type Story = StoryObj<typeof meta>;

const Rest: Story = {};
const RestLight: Story = { globals: { theme: "light" } };
const KeyboardFocus: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    // One ring: a pointer over a neighbour draws none while the keyboard holds this one.
    await userEvent.hover(within(canvasElement).getByRole("button", { name: "Go live" }));
    await expect(within(canvasElement).getAllByTestId("ring-actionFocus")).toHaveLength(1);
    await expect(within(canvasElement).queryByTestId("ring-contentPrimary")).toBeNull();
  },
};
const Pointed: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.hover(within(canvasElement).getByRole("button", { name: "Go live" }));
    await expect(within(canvasElement).getByTestId("ring-contentPrimary")).toBeVisible();
  },
};

export { KeyboardFocus, Pointed, Rest, RestLight };
export default meta;
