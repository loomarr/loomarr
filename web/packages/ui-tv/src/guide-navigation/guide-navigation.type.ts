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
