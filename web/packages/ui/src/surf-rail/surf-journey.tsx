import type { GuideController } from "@loomarr/core/guide";
import type { Density } from "@loomarr/design-system";
import { Surface } from "@loomarr/design-system";
import { useEffect, useMemo, useState, useSyncExternalStore } from "react";
import { DeviceDisconnectAction } from "../device-disconnect";
import type { FocusTargetRegistry } from "../focus-target";
import { StatePanel } from "../state-panel";
import { restoreSurfSelection, surfGroupsFromGuide } from "./surf-data";
import { SurfRail, surfIdentityLabel } from "./surf-rail";
import type { SurfGroupData, SurfRailProps, SurfSelection } from "./surf-rail.type";

interface SurfJourneyProps {
  clientName: string;
  clientVersion: string;
  controller: GuideController;
  currentChannelId?: string;
  density: Density;
  focusRegistry?: FocusTargetRegistry<SurfSelection>;
  now?: () => number;
  onDisconnect?: () => Promise<void> | void;
  onTune: (channelId: string) => void;
  playableChannelIds: readonly string[];
  recentChannelIds: readonly string[];
  renderArtwork?: SurfRailProps["renderArtwork"];
  renderChannelLogo?: SurfRailProps["renderChannelLogo"];
  restoreSelection?: (
    groups: readonly SurfGroupData[],
    selection: SurfSelection,
  ) => SurfSelection | undefined;
  serverVersion?: string;
}

const SurfJourney = ({
  clientVersion,
  clientName,
  controller,
  currentChannelId,
  density,
  focusRegistry,
  now = Date.now,
  onDisconnect,
  onTune,
  playableChannelIds,
  recentChannelIds,
  renderArtwork,
  renderChannelLogo,
  restoreSelection = restoreSurfSelection,
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
  const requestedSelection = selection ?? preferredSelection;
  const resolvedSelection = requestedSelection
    ? restoreSelection(groups, requestedSelection)
    : restoreSurfSelection(groups);
  const resolvedChannelId = resolvedSelection?.channelId;
  const resolvedGroup = resolvedSelection?.group;
  useEffect(() => {
    if (!resolvedChannelId || !resolvedGroup) return;
    focusRegistry?.request({ channelId: resolvedChannelId, group: resolvedGroup });
  }, [focusRegistry, resolvedChannelId, resolvedGroup]);

  if (snapshot.status !== "ready" || !resolvedSelection) {
    const kind = snapshot.status === "error" ? "error" : snapshot.status === "empty" ? "empty" : "loading";
    return (
      <Surface
        alignItems="center"
        borderRadius={0}
        borderWidth={0}
        flex={1}
        gap="$control"
        justifyContent="center"
        level="canvas"
      >
        <StatePanel
          action={
            kind === "error"
              ? { label: "Try again", onPress: () => void controller.refresh(currentChannelId) }
              : undefined
          }
          compact={density === "tv"}
          density={density}
          description={
            kind === "error"
              ? "The latest channel schedule could not be loaded."
              : kind === "empty"
                ? "No playable channels are available yet."
                : "Reading the latest channels."
          }
          kind={kind}
          metadata={surfIdentityLabel(clientName, clientVersion, serverVersion)}
          title={
            kind === "error"
              ? "Surf unavailable"
              : kind === "empty"
                ? "No channels on air"
                : "Loading channels"
          }
        />
        {onDisconnect ? (
          <DeviceDisconnectAction
            density={density}
            onDisconnect={onDisconnect}
            preferredFocus={density === "tv" && kind !== "error"}
          />
        ) : null}
      </Surface>
    );
  }

  return (
    <SurfRail
      clientName={clientName}
      clientVersion={clientVersion}
      currentChannelId={currentChannelId}
      density={density}
      focusRegistry={focusRegistry}
      groups={groups}
      onFocusSelection={setSelection}
      onDisconnect={onDisconnect}
      onTune={onTune}
      renderArtwork={renderArtwork}
      renderChannelLogo={renderChannelLogo}
      selection={resolvedSelection}
      serverVersion={serverVersion}
    />
  );
};

export type { SurfJourneyProps };
export { SurfJourney };
