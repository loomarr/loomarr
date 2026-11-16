import { Reveal, Stack, Text, useRevealOnHover } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// Reveal — a line or control that fades in while its row is hovered or holds focus: the "changed by"
// line under a setting, the play disc on a clip. It stays in the tree when hidden, so Tab reaches a
// control inside it and a screen reader reads it. The row spreads `rowProps` and hands `shown` on.
const Row = ({ label }: { label: string }) => {
  const { rowProps, shown } = useRevealOnHover();
  return (
    <div {...rowProps} style={{ display: "flex", flexDirection: "column", gap: 6, maxWidth: 360 }}>
      <Text as="label" htmlFor={label} textRole="label">
        {label}
      </Text>
      <input id={label} style={{ height: 36 }} />
      <Reveal render="p" shown={shown}>
        <Text as="span" textRole="hint" tone="secondary">
          Changed by Sam, 9 days ago
        </Text>
      </Reveal>
    </div>
  );
};

const Workshop = () => (
  <Stack gap="6" padding={24}>
    <Row label="Server address" />
    <Row label="Sign-in lifetime" />
  </Stack>
);

const meta = {
  title: "Loomarr Components/Reveal",
  component: Workshop,
} satisfies Meta<typeof Workshop>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Light: Story = { globals: { theme: "light" } };
// Tab into the first field: focus counts as hover, so the line shows.
const Focused: Story = {
  play: async ({ userEvent }) => {
    await userEvent.tab();
  },
};

export default meta;
export { Default, Focused, Light };
