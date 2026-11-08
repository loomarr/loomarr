import { CompactSelect, Screen, Surface, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

// CompactSelect — the toolbar and settings-row choice the mocks draw as a native <select>: the
// field select's closed box, with the browser's own list.
const ranges = [
  { label: "All retained", value: "all" },
  { label: "Last hour", value: "1h" },
  { label: "Last 6 hours", value: "6h" },
  { label: "Last 24 hours", value: "24h" },
] as const;

const orders = [
  { label: "Newest first", value: "newest" },
  { label: "Oldest first", value: "oldest" },
] as const;

const Toolbar = ({ disabled = false, invalid = false }: { disabled?: boolean; invalid?: boolean }) => {
  const [range, setRange] = useState<(typeof ranges)[number]["value"]>("1h");
  const [order, setOrder] = useState<(typeof orders)[number]["value"]>("newest");
  return (
    <Screen gap="section">
      <Surface gap="control" padding="section">
        <Text textRole="title">Log filters</Text>
        <div style={{ alignItems: "center", display: "flex", flexWrap: "wrap", gap: 12 }}>
          <CompactSelect
            disabled={disabled}
            invalid={invalid}
            label="Time range"
            onValueChange={setRange}
            options={ranges}
            value={range}
            width={160}
          />
          <CompactSelect
            disabled={disabled}
            label="Log order"
            onValueChange={setOrder}
            options={orders}
            value={order}
            width={160}
          />
        </div>
      </Surface>
      <Surface gap="control" padding="section">
        <Text textRole="title">Full row</Text>
        <CompactSelect label="Log order" onValueChange={setOrder} options={orders} value={order} />
      </Surface>
    </Screen>
  );
};

const meta = {
  title: "Loomarr Components/CompactSelect",
  component: Toolbar,
} satisfies Meta<typeof Toolbar>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Invalid: Story = { args: { invalid: true } };
const Disabled: Story = { args: { disabled: true } };

export default meta;
export { Default, Disabled, Invalid };
