import {
  type GuideLayout,
  type GuideNavigationDirection,
  guideSelectionForChannel,
  moveGuideSelection,
} from "@loomarr/core/guide";

import type {
  TvGuideActivation,
  TvGuideFilterOption,
  TvGuideMoveResult,
  TvGuideNavigationState,
  TvGuideRowWindow,
} from "./guide-navigation.type";

const enabledFilters = (filters: readonly TvGuideFilterOption[]) =>
  filters.filter((filter) => !filter.disabled);

const moveFilterFocus = (
  state: TvGuideNavigationState,
  direction: GuideNavigationDirection,
  filters: readonly TvGuideFilterOption[],
): TvGuideMoveResult => {
  if (direction === "down") {
    return { state: { ...state, focus: { region: "grid", selection: state.gridSelection } } };
  }
  if (direction === "up") return { boundary: "up", state };

  const options = enabledFilters(filters);
  const current = state.focus.region === "filters" ? state.focus.filter : state.activeFilter;
  const currentIndex = Math.max(
    0,
    options.findIndex((filter) => filter.value === current),
  );
  const nextIndex = currentIndex + (direction === "left" ? -1 : 1);
  const next = options[nextIndex];
  if (!next) return { boundary: direction, state };
  return { state: { ...state, focus: { filter: next.value, region: "filters" } } };
};

const moveTvGuideFocus = (
  layout: GuideLayout,
  state: TvGuideNavigationState,
  direction: GuideNavigationDirection,
  filters: readonly TvGuideFilterOption[],
): TvGuideMoveResult => {
  if (state.focus.region === "filters") return moveFilterFocus(state, direction, filters);

  const movement = moveGuideSelection(layout, state.focus.selection, direction);
  if (movement.boundary === "up") {
    const options = enabledFilters(filters);
    const filter = options.find((option) => option.value === state.activeFilter)?.value ?? options[0]?.value;
    if (filter) return { state: { ...state, focus: { filter, region: "filters" } } };
  }
  if (movement.boundary) return { boundary: movement.boundary, state };
  return {
    state: {
      ...state,
      focus: { region: "grid", selection: movement.selection },
      gridSelection: movement.selection,
    },
  };
};

const activateTvGuideFocus = (state: TvGuideNavigationState): TvGuideActivation =>
  state.focus.region === "filters"
    ? { filter: state.focus.filter, kind: "filter" }
    : { kind: "tune", selection: state.focus.selection };

const restoreTvGuideFocus = (
  layout: GuideLayout,
  state: TvGuideNavigationState,
): TvGuideNavigationState | undefined => {
  const preferred = guideSelectionForChannel(
    layout,
    state.gridSelection.channelId,
    state.gridSelection.anchorMs,
  );
  const first = layout.channels[0]
    ? guideSelectionForChannel(layout, layout.channels[0].source.channelId, state.gridSelection.anchorMs)
    : undefined;
  const selection = preferred ?? first;
  if (!selection) return undefined;
  return {
    ...state,
    focus: state.focus.region === "grid" ? { region: "grid", selection } : state.focus,
    gridSelection: selection,
  };
};

const tvGuideRowWindow = (
  channelCount: number,
  focusedIndex: number,
  visibleRows: number,
  overscanRows = 2,
): TvGuideRowWindow => {
  const count = Math.max(0, channelCount);
  const visible = Math.max(1, visibleRows);
  const focus = Math.min(Math.max(0, focusedIndex), Math.max(0, count - 1));
  const viewportStart = Math.min(Math.max(0, focus - Math.floor(visible / 2)), Math.max(0, count - visible));
  return {
    end: Math.min(count, viewportStart + visible + overscanRows),
    positionLabel: count === 0 ? "No channels" : `${focus + 1} of ${count}`,
    start: Math.max(0, viewportStart - overscanRows),
  };
};

export { activateTvGuideFocus, moveTvGuideFocus, restoreTvGuideFocus, tvGuideRowWindow };
