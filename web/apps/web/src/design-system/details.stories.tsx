import { Details, Stack, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// Details — the bare disclosure the diagnostics, filler and AI pages use: a native marker and a
// summary line, with the content below it. It is a native `<details>`, so Enter and Space open it,
// it can start open, and it reports its toggle. It is not `Disclosure`, the raised card with a chevron.
const Workshop = ({ width = 480 }: { width?: number }) => (
  <Stack gap="6" maxWidth={width} padding={24}>
    <Details summary="Technical details" textRole="cardLabel">
      <Stack gap="2" marginTop={12}>
        <Text textRole="compact" tone="secondary">
          Request ID 7f3a-91c2, attempt 2 of 3.
        </Text>
      </Stack>
    </Details>
    <Details summary="More filters" textRole="compact" tone="secondary">
      <Stack marginTop={8}>
        <Text textRole="compact">Channel, severity and source.</Text>
      </Stack>
    </Details>
    <Details defaultOpen summary="Raw search data" textRole="hint">
      <Stack marginTop={8}>
        <Text textRole="mono">{"{ query: 'sunrise', results: 3 }"}</Text>
      </Stack>
    </Details>
    <Details summary={(open) => (open ? "Hide error" : "Show error")} textRole="hint" tone="danger">
      <Stack marginTop={4}>
        <Text textRole="mono">connection refused: tuner 2</Text>
      </Stack>
    </Details>
    <Details fitSummary marker={false} summary="Customize report" textRole="cardLabel">
      <Stack marginTop={8}>
        <Text textRole="compact">Choose what the report includes.</Text>
      </Stack>
    </Details>
  </Stack>
);

const meta = {
  title: "Loomarr Components/Details",
  component: Workshop,
} satisfies Meta<typeof Workshop>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Light: Story = { globals: { theme: "light" } };
const Narrow: Story = { args: { width: 260 } };
const Opened: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByText("Technical details"));
    await userEvent.click(canvas.getByText("Show error"));
  },
};
const Focused: Story = {
  // Tab, so the browser counts the focus as the keyboard's and draws the amber outline.
  play: async ({ userEvent }) => {
    await userEvent.tab();
  },
};

export default meta;
export { Default, Focused, Light, Narrow, Opened };
