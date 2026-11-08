import { DateField, Screen, Surface, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

// DateField — the browser's own date and date-time inputs in the Field's box.
const Dates = ({ disabled = false }: { disabled?: boolean }) => {
  const [airDate, setAirDate] = useState("1989-09-16");
  const [at, setAt] = useState("2026-11-08T21:30");
  return (
    <Screen gap="section">
      <Surface gap="control" padding="section">
        <Text textRole="title">Dates</Text>
        <DateField
          description="When it first aired, if you know."
          disabled={disabled}
          label="Air date (optional)"
          onValueChange={setAirDate}
          value={airDate}
        />
        <DateField
          disabled={disabled}
          kind="datetime"
          label="At"
          onValueChange={setAt}
          value={at}
          width={224}
        />
        <DateField disabled={disabled} label="Empty" onValueChange={() => undefined} value="" width={224} />
      </Surface>
    </Screen>
  );
};

const meta = {
  title: "Loomarr Components/DateField",
  component: Dates,
} satisfies Meta<typeof Dates>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Disabled: Story = { args: { disabled: true } };

export default meta;
export { Default, Disabled };
