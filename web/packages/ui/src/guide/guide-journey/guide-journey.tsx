import { guideSelectionForChannel } from "@loomarr/core/guide";
import { Notice, Surface } from "@loomarr/design-system";
import type { ReactNode } from "react";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";

import { GuideExperience } from "../guide";
import type { GuideFilter } from "../guide.type";
import { GuideCompact } from "../guide-compact";
import { guideFilterChannelIds, guideFilterOptions } from "../guide-filter";
import type { GuideJourneyProps } from "./guide-journey.type";

const GuideJourney = ({
  channelWindow,
  controller,
  density = "pointer",
  dock,
  focusRegistry,
  myChannels,
  onTimeEdge,
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

  // The D-pad walks the controller's rows, so a TV or desktop filter restricts them; the phone's
  // grid is tapped, not walked, and filters its own rows.
  const compact = density === "touch";
  useEffect(() => {
    controller.restrict(compact ? undefined : restrictTo);
  }, [compact, controller, restrictTo]);

  // The phone's grid still holds every channel, so a filter that hides the selected channel leaves
  // the dock describing a programme that isn't drawn: re-pick the first channel the filter keeps.
  const layout = snapshot.layout;
  const selection = snapshot.selection;
  useEffect(() => {
    if (!compact || !restrictTo || !layout || !selection || restrictTo.includes(selection.channelId)) return;
    const kept = layout.channels.find(({ source }) => restrictTo.includes(source.channelId));
    if (!kept) return;
    const repick = guideSelectionForChannel(layout, kept.source.channelId, selection.anchorMs);
    if (repick) controller.select(repick);
  }, [compact, controller, layout, restrictTo, selection]);

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

  // The phone's grid draws the now line against a clock that moves with the minute.
  const [nowMs, setNowMs] = useState(Date.now);
  useEffect(() => {
    if (!compact) return;
    const timer = setInterval(() => setNowMs(Date.now()), 30_000);
    return () => clearInterval(timer);
  }, [compact]);

  let content: ReactNode;
  if (snapshot.status !== "ready" || !snapshot.layout || !snapshot.selection) {
    content = (
      <GuideExperience
        density={density}
        onRetry={
          snapshot.status === "error"
            ? () => {
                if (snapshot.refreshing) return;
                retryPressed.current = true;
                void controller.refresh(preferredChannelId);
              }
            : undefined
        }
        retrying={snapshot.status === "error" && snapshot.refreshing === true}
        state={snapshot.status === "ready" ? "error" : snapshot.status}
      />
    );
  } else if (compact) {
    // The phone (#1659 mocks 5d/5f): the compact grid keeps every channel and filters its own rows.
    content = (
      <GuideCompact
        density={density}
        dock={dock}
        filter={filter}
        layout={snapshot.layout}
        myChannels={myChannels}
        nowMs={nowMs}
        onFilterChange={setFilter}
        onSelect={controller.select}
        onWatch={onTune}
        renderArtwork={renderArtwork}
        selection={snapshot.selection}
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
        onTimeEdge={onTimeEdge}
        onTune={(selection) => onTune(selection.channelId)}
        renderArtwork={renderArtwork}
        renderChannelLogo={renderChannelLogo}
        selection={snapshot.selection}
      />
    );
  }

  // A failed background refresh keeps the layout on screen, an empty one included; say so, so a stale
  // grid (or a stale "No channels on air") doesn't read as current.
  const stale =
    snapshot.error !== undefined &&
    (snapshot.status === "ready" || snapshot.status === "empty") &&
    snapshot.layout !== undefined;

  // A retry that works takes the notice (or the error panel) away, and the focus on its button with
  // it. The Guide's home, the selected programme, is where a remote goes instead of nowhere.
  const retryPressed = useRef(false);
  // The press is for that one retry: whichever way it settles, a later recovery that nobody pressed
  // for (a channel frame's refresh, a tune) leaves the viewer's focus where it is.
  const recovered = snapshot.error === undefined && snapshot.status === "ready";
  const refreshing = snapshot.refreshing === true;
  useEffect(() => {
    if (!retryPressed.current) return;
    if (!recovered) {
      if (!refreshing) retryPressed.current = false;
      return;
    }
    retryPressed.current = false;
    if (selectedAnchorMs === undefined || selectedChannelId === undefined) return;
    if (selectedScheduleBlockId === undefined) return;
    focusRegistry?.request({
      kind: "airing",
      selection: {
        anchorMs: selectedAnchorMs,
        channelId: selectedChannelId,
        scheduleBlockId: selectedScheduleBlockId,
      },
    });
  }, [focusRegistry, recovered, refreshing, selectedAnchorMs, selectedChannelId, selectedScheduleBlockId]);

  return (
    <Surface borderRadius={0} borderWidth={0} flex={1} level="canvas">
      {stale ? (
        <Surface backgroundColor="transparent" borderWidth={0} padding="control">
          <Notice
            // The notice stays mounted while the retry runs, so the viewer's focus stays on its
            // button; a press during the retry is the same retry, not another request.
            action={{
              label: snapshot.refreshing ? "Trying again…" : "Try again",
              onPress: () => {
                if (snapshot.refreshing) return;
                retryPressed.current = true;
                void controller.refresh(preferredChannelId);
              },
            }}
            body="Showing what Loomarr had a moment ago."
            density={density}
            title="Couldn't refresh the Guide"
            tone="warn"
          />
        </Surface>
      ) : null}
      {content}
    </Surface>
  );
};

export { GuideJourney };
