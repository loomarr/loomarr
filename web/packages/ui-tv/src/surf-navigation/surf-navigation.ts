import type { SurfGroupData, SurfSelection } from "@loomarr/ui";

const surfSelections = (groups: readonly SurfGroupData[]): SurfSelection[] =>
  groups.flatMap((group) => group.channels.map((channel) => ({ channelId: channel.id, group: group.kind })));

const restoreTvSurfSelection = (
  groups: readonly SurfGroupData[],
  selection: SurfSelection,
): SurfSelection | undefined => {
  const selections = surfSelections(groups);
  return (
    selections.find(
      (candidate) => candidate.group === selection.group && candidate.channelId === selection.channelId,
    ) ??
    selections.find((candidate) => candidate.channelId === selection.channelId) ??
    selections[0]
  );
};

/** The channel a long-press of Select acts on: the focused channel card's. */
const tvSurfFavouriteChannel = (selection: SurfSelection | undefined): string | undefined =>
  selection?.channelId;

export { restoreTvSurfSelection, tvSurfFavouriteChannel };
