import type { GuideController } from "@loomarr/core/guide";
import type { Density } from "@loomarr/design-system";
import { Surface } from "@loomarr/design-system";
import { useEffect, useMemo, useState, useSyncExternalStore } from "react";
import { StatePanel } from "../state-panel";
import { restoreSurfSelection, surfGroupsFromGuide } from "./surf-data";
import { SurfRail } from "./surf-rail";
import type { SurfSelection } from "./surf-rail.type";

interface SurfJourneyProps {
  clientName: string;
  clientVersion: string;
  controller: GuideController;
  currentChannelId?: string;
  density: Density;
  now?: () => number;
  onTune: (channelId: string) => void;
  playableChannelIds: readonly string[];
  recentChannelIds: readonly string[];
  serverVersion?: string;
}

const SurfJourney = ({
  clientVersion,
  clientName,
  controller,
  currentChannelId,
  density,
  now = Date.now,
  onTune,
  playableChannelIds,
  recentChannelIds,
  serverVersion,
}: SurfJourneyProps) => {
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const [selection, setSelection] = useState<SurfSelection>();
  useEffect(() => {
    void controller.refresh(currentChannelId);
  }, [controller, currentChannelId]);
  const groups = useMemo(
    () =>
      snapshot.layout
        ? surfGroupsFromGuide(snapshot.layout, playableChannelIds, recentChannelIds, now())
        : [],
    [now, playableChannelIds, recentChannelIds, snapshot.layout],
  );
  const preferredSelection = currentChannelId
    ? { channelId: currentChannelId, group: "recent" as const }
    : undefined;
  const resolvedSelection = restoreSurfSelection(groups, selection ?? preferredSelection);

  if (snapshot.status !== "ready" || !resolvedSelection) {
    const kind = snapshot.status === "error" ? "error" : snapshot.status === "empty" ? "empty" : "loading";
    return (
      <Surface borderRadius={0} borderWidth={0} flex={1} justifyContent="center" level="canvas">
        <StatePanel
          action={
            kind === "error"
              ? { label: "Try again", onPress: () => void controller.refresh(currentChannelId) }
              : undefined
          }
          density={density}
          description={
            kind === "error"
              ? "The latest channel schedule could not be loaded."
              : kind === "empty"
                ? "No playable channels are available yet."
                : "Reading the latest channels."
          }
          kind={kind}
          title={
            kind === "error"
              ? "Surf unavailable"
              : kind === "empty"
                ? "No channels on air"
                : "Loading channels"
          }
        />
      </Surface>
    );
  }

  return (
    <SurfRail
      clientName={clientName}
      clientVersion={clientVersion}
      density={density}
      groups={groups}
      onFocusSelection={setSelection}
      onTune={onTune}
      selection={resolvedSelection}
      serverVersion={serverVersion}
    />
  );
};

export type { SurfJourneyProps };
export { SurfJourney };
