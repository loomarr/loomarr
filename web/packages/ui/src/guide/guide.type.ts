import type { GuideAiringLayout, GuideLayout, GuideSelection } from "@loomarr/core/guide";
import type { Density } from "@loomarr/design-system";
import type { ReactNode } from "react";

import type { FocusTargetRegistry } from "../focus-target";

type GuideFilter = "all" | "favourites" | "recent";

type GuideFocusTarget =
  | { filter: GuideFilter; kind: "filter" }
  | { kind: "airing"; selection: GuideSelection };

type GuideFilterOption = {
  /** How many channels the filter shows; drawn after the label ("Recent · 4"). */
  count?: number;
  disabled?: boolean;
  label: string;
  value: GuideFilter;
};

type GuideArtworkRenderer = (airing: GuideAiringLayout) => ReactNode;
type GuideLogoRenderer = (channel: GuideLayout["channels"][number]) => ReactNode;

type GuideChannelWindow = {
  end: number;
  positionLabel: string;
  start: number;
};

interface GuideSurfaceProps {
  density?: Density;
  channelWindow?: GuideChannelWindow;
  filter?: GuideFilter;
  filters?: readonly GuideFilterOption[];
  focusRegistry?: FocusTargetRegistry<GuideFocusTarget>;
  layout: GuideLayout;
  onFilterChange?: (filter: GuideFilter) => void;
  onSelectionChange: (selection: GuideSelection) => void;
  /**
   * TV only: the D-pad pressed past the timeline's left or right edge, from the last focused
   * cell. The handler decides whether that pages the window (#1659 decision N4) and where focus goes.
   */
  onTimeEdge?: (side: "left" | "right") => void;
  onTune?: (selection: GuideSelection) => void;
  renderArtwork?: GuideArtworkRenderer;
  renderChannelLogo?: GuideLogoRenderer;
  selection: GuideSelection;
}

type GuideUnavailableState = "empty" | "error" | "loading" | "offline";
type GuideReadyProps = GuideSurfaceProps & { state?: "ready" };

interface GuideUnavailableProps {
  density?: Density;
  onRetry?: () => void;
  state: GuideUnavailableState;
}

type GuideExperienceProps = GuideReadyProps | GuideUnavailableProps;

export type {
  GuideArtworkRenderer,
  GuideChannelWindow,
  GuideExperienceProps,
  GuideFilter,
  GuideFilterOption,
  GuideFocusTarget,
  GuideLogoRenderer,
  GuideSurfaceProps,
  GuideUnavailableState,
};
