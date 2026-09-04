import { createGuideController } from "@loomarr/core";
import { guideChannels, guideFrom, guideNow, guideTo } from "@loomarr/fixtures";
import { GuideJourney } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";
import { useEffect, useMemo } from "react";

type GuideScenario = "empty" | "error" | "loading" | "ready";

const GuideJourneyWorkshop = ({
  density = "touch",
  scenario = "ready",
}: {
  density?: "touch" | "tv";
  scenario?: GuideScenario;
}) => {
  const controller = useMemo(
    () =>
      createGuideController({
        now: () => guideNow,
        source: {
          load: async (_signal) => {
            if (scenario === "error") throw new Error("The guide could not be loaded.");
            if (scenario === "loading") return new Promise(() => undefined);
            return {
              channels: scenario === "empty" ? [] : guideChannels,
              fromMs: guideFrom,
              timezone: "America/New_York",
              toMs: guideTo,
            };
          },
        },
      }),
    [scenario],
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
const TvEmpty: Story = { args: { density: "tv", scenario: "empty" } };
const TvError: Story = { args: { density: "tv", scenario: "error" } };
const TvLoading: Story = { args: { density: "tv", scenario: "loading" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Touch, Tv, TvEmpty, TvError, TvLoading };
