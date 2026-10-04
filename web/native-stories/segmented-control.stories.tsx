import { SegmentedControl, type SegmentOption, Surface } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-native";
import { useState } from "react";

// The iPhone segmented control, on its own: Requests uses three segments for an admin and two for a
// member. Every story is live, so tapping a segment moves the thumb.
const requestSegments = [
  { count: 6, label: "Needs you", value: "needs-you" },
  { count: 3, label: "In progress", value: "in-progress" },
  { count: 2, label: "Done", value: "done" },
] as const satisfies readonly SegmentOption<string>[];

const SegmentedWorkshop = ({
  options = requestSegments,
  start = "needs-you",
}: {
  options?: readonly SegmentOption<string>[];
  start?: string;
}) => {
  const [value, setValue] = useState(start);
  return (
    <Surface backgroundColor="$transparent" borderWidth={0} padding="$control">
      <SegmentedControl
        accessibilityLabel="Requests sections"
        onValueChange={setValue}
        options={options}
        value={value}
      />
    </Surface>
  );
};

const meta = {
  title: "Loomarr Foundations/Segmented Control",
  component: SegmentedWorkshop,
} satisfies Meta<typeof SegmentedWorkshop>;

type Story = StoryObj<typeof meta>;
const ThreeSegments: Story = {};
const TwoSegments: Story = { args: { options: requestSegments.slice(1), start: "in-progress" } };
const NoCounts: Story = {
  args: { options: requestSegments.map(({ label, value }) => ({ label, value })) },
};
const OverflowCounts: Story = {
  args: { options: requestSegments.map((segment) => ({ ...segment, count: 128 })) },
};
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, NoCounts, OverflowCounts, ThreeSegments, TwoSegments };
