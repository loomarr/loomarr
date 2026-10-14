import type { GuideNavigationDirection, GuideSelection } from "@loomarr/core/guide";

type TvGuideFilterOption = {
  disabled?: boolean;
  value: string;
};

type TvGuideFocus = { filter: string; region: "filters" } | { region: "grid"; selection: GuideSelection };

type TvGuideNavigationState = {
  activeFilter: string;
  focus: TvGuideFocus;
  gridSelection: GuideSelection;
};

type TvGuideMoveResult = {
  boundary?: GuideNavigationDirection;
  /**
   * Set only on a left/right boundary (#1659 decision N4): "earlier"/"later" asks the platform to
   * page the served time window; absent means the edge truly has nowhere to go (LEFT already at
   * now, or RIGHT off a programme that doesn't span the whole window).
   */
  pageIntent?: "earlier" | "later";
  state: TvGuideNavigationState;
};

type TvGuideActivation = { filter: string; kind: "filter" } | { kind: "tune"; selection: GuideSelection };

type TvGuideRowWindow = {
  end: number;
  positionLabel: string;
  start: number;
};

export type {
  TvGuideActivation,
  TvGuideFilterOption,
  TvGuideFocus,
  TvGuideMoveResult,
  TvGuideNavigationState,
  TvGuideRowWindow,
};
