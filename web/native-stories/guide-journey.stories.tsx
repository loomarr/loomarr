import { createGuideController } from "@loomarr/core";
import { guideChannels, guideFrom, guideNow, guideTo } from "@loomarr/fixtures";
import { GuideJourney } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";
import { useEffect, useMemo } from "react";

const GuideJourneyWorkshop = ({ density = "touch" }: { density?: "touch" | "tv" }) => {
  const controller = useMemo(
    () =>
      createGuideController({
        now: () => guideNow,
        source: {
          load: async () => ({
            channels: guideChannels,
            fromMs: guideFrom,
            timezone: "America/New_York",
            toMs: guideTo,
          }),
        },
      }),
    [],
  );
  useEffect(() => () => controller.dispose(), [controller]);

  return (
    <GuideJourney
      controller={controller}
      density={density}
      onTune={() => undefined}
      preferredChannelId="ch-springfield"
    />
  );
};

const meta = {
  title: "Loomarr Components/Guide Journey",
  component: GuideJourneyWorkshop,
} satisfies Meta<typeof GuideJourneyWorkshop>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Touch, Tv };
