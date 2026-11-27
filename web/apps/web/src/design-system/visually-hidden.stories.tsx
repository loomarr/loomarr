import { Stack, Text, VisuallyHidden } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// VisuallyHidden — words for a screen reader that the page does not print: a heading a picture
// replaces, a table's caption, a label a glyph stands in for. It is clipped to a pixel, not
// `display: none`, so it stays in the accessibility tree. The story draws nothing visible on purpose:
// the first line is the control and the hidden words sit between it and the second.
const Workshop = () => (
  <Stack gap="2" padding={24}>
    <Text textRole="compact">Before the hidden words</Text>
    <VisuallyHidden>Read by a screen reader only</VisuallyHidden>
    <Text textRole="compact">After the hidden words</Text>
  </Stack>
);

const meta = {
  title: "Loomarr Components/VisuallyHidden",
  component: Workshop,
} satisfies Meta<typeof Workshop>;

type Story = StoryObj<typeof meta>;

const Default: Story = {
  play: async ({ canvas, expect }) => {
    await expect(canvas.getByText("Read by a screen reader only")).toBeInTheDocument();
  },
};

export default meta;
export { Default };
