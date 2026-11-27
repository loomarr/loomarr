import { SkipLink, Stack, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// SkipLink — the first Tab stop of a page whose chrome puts a long run of controls before its content.
// It is clipped to a pixel until it holds focus, then reveals itself top left as the amber chip.
const Workshop = () => (
  <div style={{ minHeight: 160, position: "relative" }}>
    <SkipLink href="#story-main">Skip to content</SkipLink>
    <Stack gap="2" padding={24}>
      <Text as="p" textRole="compact" tone="secondary">
        Press Tab: the link appears at the top left of this frame.
      </Text>
      <button type="button">Rail item</button>
    </Stack>
    <main id="story-main" tabIndex={-1} />
  </div>
);

const meta = {
  title: "Loomarr Components/SkipLink",
  component: Workshop,
} satisfies Meta<typeof Workshop>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Focused: Story = {
  play: async ({ userEvent }) => {
    await userEvent.tab();
  },
};
const FocusedLight: Story = { ...Focused, globals: { theme: "light" } };

export default meta;
export { Default, Focused, FocusedLight };
