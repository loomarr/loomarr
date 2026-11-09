import {
  normalizeYearRanges,
  type YearRange,
  YearRangeListField,
  type YearRangeListFieldProps,
} from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

// YearRangeListField — a list of whole-year ranges: a From/To row per range and a blank row to add
// another. The label names every box and button, so a screen with several lists stays navigable.

// The list lives in state so adding, editing and removing a row works in the canvas.
const ListStage = (args: YearRangeListFieldProps) => {
  const [ranges, setRanges] = useState<YearRange[]>(args.ranges);
  return (
    <YearRangeListField {...args} onChange={(next) => setRanges(normalizeYearRanges(next))} ranges={ranges} />
  );
};

const meta = {
  title: "Loomarr Components/YearRangeListField",
  component: ListStage,
  args: { label: "Movie release", onChange: () => undefined, ranges: [] },
  decorators: [
    (Story) => (
      <div style={{ padding: 24, width: 420 }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof ListStage>;

type Story = StoryObj<typeof meta>;

const Empty: Story = { args: { emptyText: "No date requirement." } };
const WithRanges: Story = {
  args: {
    ranges: [
      { from: 1980, to: 1989 },
      { from: 1995, to: 1999 },
    ],
  },
};
const Disabled: Story = { args: { disabled: true, ranges: [{ from: 1990, to: 1999 }] } };
const AtTheLimit: Story = {
  args: {
    maxRanges: 2,
    ranges: [
      { from: 1960, to: 1969 },
      { from: 1980, to: 1989 },
    ],
  },
};

export default meta;
export { AtTheLimit, Disabled, Empty, WithRanges };
