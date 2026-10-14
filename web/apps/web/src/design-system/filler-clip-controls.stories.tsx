import { Surface, Text } from "@loomarr/design-system";
import { ClipSelect, type ClipSelectProps, NeedsDetailsChip } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

// The Filler Library's two clip controls (#1817, decided on #1908), drawn the way a card shows
// them: the bulk-select box beside the clip's name, and the caution "Needs details" exception on a
// ready commercial missing a match tag. Invented fixtures only.

// Selection is the control's whole job, so the story owns it the way the Library does.
const ClipControlsStage = ({
  clipName,
  needsDetails,
  selected,
}: ClipSelectProps & { needsDetails: boolean }) => {
  const [on, setOn] = useState(selected);
  return (
    <Surface gap="$control" level="raised" padding="$control" width={260}>
      <Surface
        alignItems="center"
        backgroundColor="$transparent"
        borderWidth={0}
        flexDirection="row"
        gap="$inline"
      >
        <ClipSelect clipName={clipName} onSelectedChange={setOn} selected={on} />
        <Text numberOfLines={1} textRole="label">
          {clipName}
        </Text>
      </Surface>
      {needsDetails ? <NeedsDetailsChip /> : null}
    </Surface>
  );
};

const meta = {
  title: "Loomarr Components/Filler Clip Controls",
  component: ClipControlsStage,
  decorators: [
    (Story) => (
      <div style={{ boxSizing: "border-box", minHeight: "100vh", padding: 48 }}>
        <Story />
      </div>
    ),
  ],
  args: {
    clipName: "Sunbeam Juice — Morning Glow",
    needsDetails: false,
    onSelectedChange: () => undefined,
    selected: false,
  },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ClipControlsStage>;

type Story = StoryObj<typeof meta>;

const Unselected: Story = {};
const Selected: Story = { args: { selected: true } };
// A ready commercial with no era, audience or category: the Library's one exception on a ready clip.
const NeedsDetails: Story = { args: { needsDetails: true } };
const Light: Story = { args: { needsDetails: true, selected: true }, globals: { theme: "light" } };

export default meta;
export { Light, NeedsDetails, Selected, Unselected };
