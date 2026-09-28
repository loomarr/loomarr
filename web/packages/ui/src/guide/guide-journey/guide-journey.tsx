import { Surface } from "@loomarr/design-system";
import type { ReactNode } from "react";
import { useEffect, useState, useSyncExternalStore } from "react";

import { GuideExperience } from "../guide";
import type { GuideFilter } from "../guide.type";
import { guideFilterChannelIds, guideFilterOptions } from "../guide-filter";
import type { GuideJourneyProps } from "./guide-journey.type";

const GuideJourney = ({
  channelWindow,
  controller,
  density = "pointer",
  focusRegistry,
  myChannels,
  onTune,
  preferredChannelId,
  renderArtwork,
  renderChannelLogo,
}: GuideJourneyProps) => {
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const [chosenFilter, setFilter] = useState<GuideFilter>("all");
  const filters = snapshot.layout ? guideFilterOptions(snapshot.layout, myChannels) : undefined;
  // A personal filter that empties (the last favourite unstarred) falls back to All.
  const filter = filters?.find((option) => option.value === chosenFilter)?.disabled ? "all" : chosenFilter;
  const restrictTo = guideFilterChannelIds(filter, myChannels);

  useEffect(() => {
    controller.restrict(restrictTo);
  }, [controller, restrictTo]);

  useEffect(() => {
    void controller.refresh(preferredChannelId);
  }, [controller, preferredChannelId]);

  const selectedAnchorMs = snapshot.selection?.anchorMs;
  const selectedChannelId = snapshot.selection?.channelId;
  const selectedScheduleBlockId = snapshot.selection?.scheduleBlockId;
  useEffect(() => {
    if (
      selectedAnchorMs === undefined ||
      selectedChannelId === undefined ||
      selectedScheduleBlockId === undefined
    )
      return;
    focusRegistry?.request({
      kind: "airing",
      selection: {
        anchorMs: selectedAnchorMs,
        channelId: selectedChannelId,
        scheduleBlockId: selectedScheduleBlockId,
      },
    });
  }, [focusRegistry, selectedAnchorMs, selectedChannelId, selectedScheduleBlockId]);

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
        channelWindow={channelWindow?.(snapshot.layout, snapshot.selection)}
        density={density}
        filter={filter}
        filters={filters}
        focusRegistry={focusRegistry}
        layout={snapshot.layout}
        onFilterChange={setFilter}
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

export { GuideJourney };
