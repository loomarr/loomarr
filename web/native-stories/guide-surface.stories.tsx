import { type GuideSelection, layoutGuide } from "@loomarr/core";
import { Surface, Text } from "@loomarr/design-system";
import { guideChannels, guideFrom, guideNow, guideTo } from "@loomarr/fixtures";
import { GuideSurface } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";
import { useState } from "react";

const layout = layoutGuide(
  { channels: guideChannels, fromMs: guideFrom, timezone: "America/New_York", toMs: guideTo },
  guideNow,
);

const Artwork = () => (
  <Surface alignItems="center" flex={1} justifyContent="center" width="100%">
    <Text textRole="label" tone="primary">
      SPRINGFIELD
    </Text>
  </Surface>
);

const GuideWorkshop = ({ density = "touch" }: { density?: "touch" | "tv" }) => {
  const [selection, setSelection] = useState<GuideSelection>({
    anchorMs: guideFrom + 11 * 60_000,
    channelId: "ch-springfield",
    scheduleBlockId: "block_springfield_bart_mother",
  });
  return (
    <GuideSurface
      density={density}
      layout={layout}
      onSelectionChange={setSelection}
      renderArtwork={(airing) =>
        airing.scheduleBlockId === "block_springfield_bart_mother" ? <Artwork /> : undefined
      }
      selection={selection}
    />
  );
};

const meta = {
  title: "Loomarr/Guide Surface",
  component: GuideWorkshop,
} satisfies Meta<typeof GuideWorkshop>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Touch, Tv };
