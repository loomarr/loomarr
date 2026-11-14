import { Grid, GridItem, Stack, Surface, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// Grid — equal columns whose count follows the window, with a gap from the spacing tokens and an
// item that can span columns (`grid grid-cols-1 gap-3 sm:grid-cols-2`, `sm:col-span-2`). It draws
// nothing of its own: the cells below are Surfaces so the arrangement can be seen.
const Cell = ({ children }: { children: string }) => (
  <Surface padding="control">
    <Text textRole="label">{children}</Text>
  </Surface>
);

const Workshop = ({ width = 720 }: { width?: number }) => (
  <Stack gap="section" style={{ maxWidth: width, padding: 24 }}>
    <Stack gap="inline">
      <Text textRole="metadata" tone="secondary">
        columns {"{ base: 1, sm: 2 }"}, gap control, the last item spans both
      </Text>
      <Grid columns={{ base: 1, sm: 2 }} gap="control">
        <Cell>Name</Cell>
        <Cell>Number</Cell>
        <Cell>Start</Cell>
        <Cell>End</Cell>
        <GridItem span={{ base: 1, sm: "full" }}>
          <Cell>Notes, across the whole row on a wide window</Cell>
        </GridItem>
      </Grid>
    </Stack>
    <Stack gap="inline">
      <Text textRole="metadata" tone="secondary">
        columns {"{ base: 2, sm: 3, lg: 4 }"}, separate row and column gaps
      </Text>
      <Grid columnGap="inline" columns={{ base: 2, sm: 3, lg: 4 }} rowGap="4">
        {["One", "Two", "Three", "Four", "Five", "Six", "Seven"].map((name) => (
          <Cell key={name}>{name}</Cell>
        ))}
      </Grid>
    </Stack>
    <Stack gap="inline">
      <Text textRole="metadata" tone="secondary">
        render="ul", items aligned to the start
      </Text>
      <Grid
        align="start"
        columns={2}
        gap="2"
        render="ul"
        style={{ listStyle: "none", margin: 0, padding: 0 }}
      >
        <li>
          <Cell>Short</Cell>
        </li>
        <li>
          <Surface padding="control" paddingVertical="section">
            <Text textRole="label">Tall</Text>
          </Surface>
        </li>
      </Grid>
    </Stack>
  </Stack>
);

const meta = {
  title: "Loomarr Components/Grid",
  component: Workshop,
} satisfies Meta<typeof Workshop>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Light: Story = { globals: { theme: "light" } };
const Narrow: Story = { args: { width: 360 } };

export default meta;
export { Default, Light, Narrow };
