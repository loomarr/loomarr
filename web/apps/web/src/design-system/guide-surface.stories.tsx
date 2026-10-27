import { type GuideSelection, layoutGuide } from "@loomarr/core";
import { Screen, Surface, Text } from "@loomarr/design-system";
import { guideChannels, guideFrom, guideNow, guideTo } from "@loomarr/fixtures";
import { GuideSurface } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

const layout = layoutGuide(
  { channels: guideChannels, fromMs: guideFrom, timezone: "America/New_York", toMs: guideTo },
  guideNow,
);

const GuideArtwork = () => (
  <Surface
    alignItems="center"
    backgroundColor="stateInfoSurface"
    borderWidth={0}
    flex={1}
    justifyContent="center"
    width="100%"
  >
    <Text textRole="label" tone="primary">
      KETTLE KIDS
    </Text>
  </Surface>
);

const GuideWorkshop = ({ density = "pointer" }: { density?: "pointer" | "touch" | "tv" }) => {
  const [selection, setSelection] = useState<GuideSelection>({
    anchorMs: guideFrom + 11 * 60_000,
    channelId: "ch-cartoons",
    scheduleBlockId: "block_cartoons_borrowed_teapot",
  });
  return (
    <Screen density={density} justifyContent="center">
      <GuideSurface
        density={density}
        layout={layout}
        onSelectionChange={setSelection}
        renderArtwork={(airing) =>
          airing.scheduleBlockId === "block_cartoons_borrowed_teapot" ? <GuideArtwork /> : undefined
        }
        selection={selection}
      />
    </Screen>
  );
};

const meta = {
  title: "Loomarr Components/Guide Surface",
  component: GuideWorkshop,
  args: { density: "pointer" },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof GuideWorkshop>;

type Story = StoryObj<typeof meta>;
const Pointer: Story = {};
const Touch: Story = { args: { density: "touch" } };
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };
const FocusedProgramme: Story = {
  play: async ({ canvas }) => {
    canvas.getByRole("button", { name: /Cartoon Classics, Kettle Kids · The Borrowed Teapot/ }).focus();
  },
};

export default meta;
export { FocusedProgramme, Light, Pointer, Touch, Tv };
