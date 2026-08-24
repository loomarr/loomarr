import {
  AdaptiveSplit,
  type Density,
  Disclosure,
  Screen,
  ScrollFrame,
  Surface,
  Text,
} from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

const ProgrammeRegion = ({ density }: { density: Density }) => (
  <Surface gap="$control" padding="$section">
    <Text density={density} textRole="metadata" tone="live">
      NOW PLAYING
    </Text>
    <Text density={density} textRole="display">
      Marge vs. the Monorail
    </Text>
    <Text density={density} textRole="body">
      A fast-talking stranger convinces Springfield to build a monorail with a suspicious windfall.
    </Text>
  </Surface>
);

const DetailRegion = ({ density }: { density: Density }) => {
  const [episodeInfo, setEpisodeInfo] = useState(false);
  const [technicalInfo, setTechnicalInfo] = useState(false);
  return (
    <Surface gap="$control" padding="$section">
      <Text density={density} textRole="title">
        Programme details
      </Text>
      <Disclosure
        density={density}
        description="Season, episode, and original air date"
        expanded={episodeInfo}
        label="Episode information"
        onExpandedChange={setEpisodeInfo}
      >
        <Text density={density} textRole="body">
          Season 4 · Episode 12 · January 14, 1993
        </Text>
      </Disclosure>
      <Disclosure
        density={density}
        description="Stream and accessibility metadata"
        expanded={technicalInfo}
        label="Technical information"
        onExpandedChange={setTechnicalInfo}
      >
        <Text density={density} textRole="body">
          1080p · Stereo · Closed captions
        </Text>
      </Disclosure>
    </Surface>
  );
};

const LayoutWorkshop = ({ density = "pointer" }: { density?: Density }) => (
  <Screen density={density}>
    <ScrollFrame density={density}>
      <Surface backgroundColor="$transparent" borderWidth={0} gap="$inline">
        <Text density={density} textRole="metadata" tone="info">
          RESPONSIVE COMPOSITION
        </Text>
        <Text density={density} textRole="title">
          One hierarchy, adapted to the available viewport.
        </Text>
      </Surface>
      <AdaptiveSplit
        density={density}
        primary={<ProgrammeRegion density={density} />}
        secondary={<DetailRegion density={density} />}
        testID="adaptive-layout"
      />
    </ScrollFrame>
  </Screen>
);

const meta = {
  title: "Loomarr Foundations/Layout",
  component: LayoutWorkshop,
  args: { density: "pointer" },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof LayoutWorkshop>;

type Story = StoryObj<typeof meta>;
const Pointer: Story = {};
const Touch: Story = { args: { density: "touch" } };
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };
const Expanded: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: /Episode information/ }));
  },
};
const Focused: Story = {
  play: async ({ canvas }) => {
    canvas.getByRole("button", { name: /Technical information/ }).focus();
  },
};

export default meta;
export { Expanded, Focused, Light, Pointer, Touch, Tv };
