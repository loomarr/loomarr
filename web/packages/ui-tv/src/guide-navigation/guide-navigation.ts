import {
  type GuideController,
  type GuideLayout,
  type GuideNavigationDirection,
  guideSelectionForChannel,
  moveGuideSelection,
} from "@loomarr/core/guide";
import type { GuideFocusTarget } from "@loomarr/ui";

import type { TvFocusRegistry } from "../focus-registry";
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

/**
 * What a LEFT/RIGHT boundary should ask the platform to do (#1659 decision N4, maintainer
 * 2026-10-03): LEFT at the first cell pages the window earlier, but never before `nowMs`; RIGHT
 * off a programme that spans the whole visible window pages forward one page. Any other edge (a
 * channel with no earlier programme in view, or a trailing partial programme) has nowhere to go.
 */
const boundaryPageIntent = (
  layout: GuideLayout,
  selection: TvGuideNavigationState["gridSelection"],
  boundary: GuideNavigationDirection,
  nowMs: number,
): "earlier" | "later" | undefined => {
  if (boundary === "left") return selection.anchorMs > nowMs ? "earlier" : undefined;
  if (boundary !== "right") return undefined;
  const channel = layout.channels.find((candidate) => candidate.source.channelId === selection.channelId);
  const airing = channel?.airings.find(
    (candidate) => candidate.scheduleBlockId === selection.scheduleBlockId,
  );
  return airing && airing.widthRatio >= 0.999 ? "later" : undefined;
};

const moveTvGuideFocus = (
  layout: GuideLayout,
  state: TvGuideNavigationState,
  direction: GuideNavigationDirection,
  filters: readonly TvGuideFilterOption[],
  nowMs: number,
): TvGuideMoveResult => {
  if (state.focus.region === "filters") return moveFilterFocus(state, direction, filters);

  const movement = moveGuideSelection(layout, state.focus.selection, direction);
  if (movement.boundary === "up") {
    const options = enabledFilters(filters);
    const filter = options.find((option) => option.value === state.activeFilter)?.value ?? options[0]?.value;
    if (filter) return { state: { ...state, focus: { filter, region: "filters" } } };
  }
  if (movement.boundary) {
    return {
      boundary: movement.boundary,
      pageIntent: boundaryPageIntent(layout, state.gridSelection, movement.boundary, nowMs),
      state,
    };
  }
  return {
    state: {
      ...state,
      focus: { region: "grid", selection: movement.selection },
      gridSelection: movement.selection,
    },
  };
};

/**
 * The TV app's answer to the Guide's `onTimeEdge` (#1659 decision N4). The running Guide moves
 * with the native focus engine, which serves every in-grid move and ▲/▼ between the top row and
 * the filters; it reaches a timeline edge stop only when no programme lies further that way. From
 * the cell focus left, `moveTvGuideFocus` decides: page the window (the reload re-focuses its
 * settled selection), or hand focus straight back because the edge has nowhere to go.
 *
 * Remote key events can't decide this: react-native-tvos sends JS only the key-up, and measured on
 * the emulator a cell's onFocus can arrive before or after it, so "did focus move?" is a race.
 */
const tvGuideTimeEdge = async (
  guide: Pick<GuideController, "getSnapshot" | "page">,
  registry: Pick<TvFocusRegistry<GuideFocusTarget>, "current" | "request">,
  side: "left" | "right",
  nowMs: number,
): Promise<"earlier" | "later" | undefined> => {
  const { layout, selection } = guide.getSnapshot();
  const current = registry.current();
  const from: GuideFocusTarget | undefined =
    current ?? (selection ? { kind: "airing", selection } : undefined);
  if (!from) return undefined;
  const intent =
    layout && from.kind === "airing"
      ? moveTvGuideFocus(
          layout,
          {
            activeFilter: "all",
            focus: { region: "grid", selection: from.selection },
            gridSelection: from.selection,
          },
          side,
          [],
          nowMs,
        ).pageIntent
      : undefined;
  if (intent) await guide.page(intent);
  const paged = guide.getSnapshot().layout;
  if (!intent || paged === layout) {
    // Nowhere to go, or a page that served nothing new (clamped at now, or a failed load).
    registry.request(from);
    return undefined;
  }
  // A whole-window programme still on in the paged window keeps focus. Any other page leaves
  // focus to the Guide, which focuses its settled selection; asking for `from` here could
  // overwrite that request with a programme the new window doesn't draw.
  const stillOn =
    from.kind === "airing" &&
    paged?.channels.some(
      ({ airings, source }) =>
        source.channelId === from.selection.channelId &&
        airings.some(({ scheduleBlockId }) => scheduleBlockId === from.selection.scheduleBlockId),
    );
  if (stillOn) registry.request(from);
  return intent;
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

export { activateTvGuideFocus, moveTvGuideFocus, restoreTvGuideFocus, tvGuideRowWindow, tvGuideTimeEdge };
