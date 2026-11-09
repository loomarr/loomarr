import { PressableRow, Screen, Surface, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

// The whole-row button (#2147): a list of things that open or pick one.
const rows = [
  { meta: "0:30 · from Archive", name: "Cereal ad, 1989", status: "Ready" },
  { meta: "0:15 · from Archive", name: "Toy commercial, 1992", status: "Checking" },
  { meta: "1:00 · from Local", name: "Station ident", status: "Ready" },
] as const;

const List = () => {
  const [open, setOpen] = useState<string>(rows[1].name);
  return (
    <Surface backgroundColor="transparent" borderColor="$borderDecorative" borderWidth={1} borderRadius={12}>
      {rows.map((row, index) => (
        <PressableRow
          divided={index < rows.length - 1}
          key={row.name}
          onPress={() => setOpen(row.name)}
          selected={open === row.name}
        >
          <Surface backgroundColor="transparent" borderWidth={0} flex={1} gap={2}>
            <Text textRole="label">{row.name}</Text>
            <Text textRole="hint" tone="muted">
              {row.meta}
            </Text>
          </Surface>
          <Text textRole="hint" tone="muted">
            {row.status}
          </Text>
        </PressableRow>
      ))}
    </Surface>
  );
};

const PressableRowWorkshop = () => (
  <Screen gap="section">
    <Surface gap="control" padding="section">
      <Text textRole="title">Rows that open an item</Text>
      <List />
    </Surface>
    <Surface gap="control" padding="section">
      <Text textRole="title">Disabled</Text>
      <PressableRow disabled onPress={() => undefined}>
        <Text textRole="label">Waiting for the scan</Text>
      </PressableRow>
    </Surface>
  </Screen>
);

const meta = {
  title: "Loomarr Components/PressableRow",
  component: PressableRowWorkshop,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof PressableRowWorkshop>;

type Story = StoryObj<typeof meta>;
const Workshop: Story = {};

export default meta;
export { Workshop };
