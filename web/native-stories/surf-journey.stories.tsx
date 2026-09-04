import { createGuideController } from "@loomarr/core";
import { guideChannels, guideFrom, guideNow, guideTo } from "@loomarr/fixtures";
import { SurfJourney } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";
import { useEffect, useMemo } from "react";

const SurfJourneyWorkshop = ({
  density = "touch",
  favorites = false,
}: {
  density?: "touch" | "tv";
  favorites?: boolean;
}) => {
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
    <SurfJourney
      clientVersion="0.2.0"
      controller={controller}
      currentChannelId="ch-springfield"
      density={density}
      favoriteChannelIds={favorites ? ["ch-scifi"] : undefined}
      now={() => guideNow}
      onTune={() => undefined}
      playableChannelIds={["ch-springfield", "ch-scifi"]}
      recentChannelIds={["ch-scifi"]}
      serverVersion="0.2.1"
    />
  );
};

const meta = {
  title: "Loomarr Components/Surf Journey",
  component: SurfJourneyWorkshop,
} satisfies Meta<typeof SurfJourneyWorkshop>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };
const AuthoritativeFavorites: Story = { args: { favorites: true } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { AuthoritativeFavorites, Light, Touch, Tv };
