import { restoreSurfSelection, type SurfGroupData, type SurfSelection } from "@loomarr/ui";

import type { TvSurfActivation, TvSurfDirection, TvSurfMoveResult } from "./surf-navigation.type";

const surfSelections = (groups: readonly SurfGroupData[]): SurfSelection[] =>
  groups.flatMap((group) => group.channels.map((channel) => ({ channelId: channel.id, group: group.kind })));

const moveTvSurfSelection = (
  groups: readonly SurfGroupData[],
  selection: SurfSelection,
  direction: TvSurfDirection,
): TvSurfMoveResult => {
  const selections = surfSelections(groups);
  const index = selections.findIndex(
    (candidate) => candidate.group === selection.group && candidate.channelId === selection.channelId,
  );
  const next = selections[index + (direction === "up" ? -1 : 1)];
  return next ? { selection: next } : { boundary: direction, selection };
};

const restoreTvSurfSelection = (
  groups: readonly SurfGroupData[],
  selection: SurfSelection,
): SurfSelection | undefined => restoreSurfSelection(groups, selection);

const activateTvSurfSelection = (selection: SurfSelection): TvSurfActivation => ({
  channelId: selection.channelId,
  kind: "tune",
});

const previousSurfChannel = (
  currentChannelId: string,
  previousChannelId: string | undefined,
  playableChannelIds: readonly string[],
): string | undefined =>
  previousChannelId &&
  previousChannelId !== currentChannelId &&
  playableChannelIds.includes(previousChannelId)
    ? previousChannelId
    : undefined;

export {
  activateTvSurfSelection,
  moveTvSurfSelection,
  previousSurfChannel,
  restoreTvSurfSelection,
  surfSelections,
};
