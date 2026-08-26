import type { GuideController } from "@loomarr/core/guide";
import type { Density } from "@loomarr/design-system";
import { Surface } from "@loomarr/design-system";
import type { ReactNode } from "react";
import { useEffect, useSyncExternalStore } from "react";
import { GuideExperience } from "./guide";
import type { GuideArtworkRenderer, GuideLogoRenderer } from "./guide.type";

interface GuideJourneyProps {
  controller: GuideController;
  density: Density;
  onTune: (channelId: string) => void;
  preferredChannelId?: string;
  renderArtwork?: GuideArtworkRenderer;
  renderChannelLogo?: GuideLogoRenderer;
}

const GuideJourney = ({
  controller,
  density,
  onTune,
  preferredChannelId,
  renderArtwork,
  renderChannelLogo,
}: GuideJourneyProps) => {
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  useEffect(() => {
    void controller.refresh(preferredChannelId);
  }, [controller, preferredChannelId]);

  let content: ReactNode;
  if (snapshot.status !== "ready" || !snapshot.layout || !snapshot.selection) {
    content = (
      <GuideExperience
        density={density}
        onRetry={snapshot.status === "error" ? () => void controller.refresh(preferredChannelId) : undefined}
        state={snapshot.status === "ready" ? "error" : snapshot.status}
      />
    );
  } else {
    content = (
      <GuideExperience
        density={density}
        layout={snapshot.layout}
        onSelectionChange={controller.select}
        onTune={(selection) => onTune(selection.channelId)}
        renderArtwork={renderArtwork}
        renderChannelLogo={renderChannelLogo}
        selection={snapshot.selection}
      />
    );
  }

  return (
    <Surface borderRadius={0} borderWidth={0} flex={1} level="canvas">
      {content}
    </Surface>
  );
};

export type { GuideJourneyProps };
export { GuideJourney };
