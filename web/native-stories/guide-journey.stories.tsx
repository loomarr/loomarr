import { createGuideController } from "@loomarr/core";
import { guideChannels, guideFrom, guideNow, guideTo } from "@loomarr/fixtures";
import { GuideJourney } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";
import { useEffect, useMemo, useState } from "react";

import { tvGuideChannels, tvGuideFrom, tvGuideNow, tvGuideTo } from "./tv-parity-fixtures";

// `stale` is a Guide that loaded once, then failed to refresh: the grid stays, under a notice.
type GuideScenario = "empty" | "error" | "loading" | "ready" | "stale";

const GuideJourneyWorkshop = ({
  density = "touch",
  scenario = "ready",
}: {
  density?: "touch" | "tv";
  scenario?: GuideScenario;
}) => {
  const tv = density === "tv";
  const channels = tv ? tvGuideChannels : guideChannels;
  const fromMs = tv ? tvGuideFrom : guideFrom;
  const nowMs = tv ? tvGuideNow : guideNow;
  const toMs = tv ? tvGuideTo : guideTo;
  const preferredChannelId = tv ? "noir" : "ch-cartoons";
  const controller = useMemo(() => {
    let loads = 0;
    return createGuideController({
      now: () => nowMs,
      source: {
        load: async (_signal) => {
          loads += 1;
          if (scenario === "error" || (scenario === "stale" && loads > 1))
            throw new Error("The guide could not be loaded.");
          if (scenario === "loading") return new Promise(() => undefined);
          return {
            channels: scenario === "empty" ? [] : channels,
            fromMs,
            timezone: "America/New_York",
            toMs,
          };
        },
      },
    });
  }, [channels, fromMs, nowMs, scenario, toMs]);
  useEffect(() => () => controller.dispose(), [controller]);

  // A stale Guide has to have loaded once before the journey's own refresh fails over it.
  const [loaded, setLoaded] = useState(scenario !== "stale");
  useEffect(() => {
    if (scenario !== "stale") return;
    void controller.refresh(preferredChannelId).then(() => setLoaded(true));
  }, [controller, preferredChannelId, scenario]);
  if (!loaded) return null;

  return (
    <GuideJourney
      controller={controller}
      density={density}
      onTune={() => undefined}
      preferredChannelId={preferredChannelId}
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
const TouchRefreshFailed: Story = { args: { scenario: "stale" } };
const TvRefreshFailed: Story = { args: { density: "tv", scenario: "stale" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Touch, TouchRefreshFailed, Tv, TvEmpty, TvError, TvLoading, TvRefreshFailed };
