import type { GuideAiringLayout, GuideLayout, GuideSelection } from "@loomarr/core/guide";
import type { Density } from "@loomarr/design-system";
import type { ReactNode } from "react";

type GuideFilter = "all" | "favourites" | "recent";

type GuideFilterOption = {
  disabled?: boolean;
  label: string;
  value: GuideFilter;
};

type GuideArtworkRenderer = (airing: GuideAiringLayout) => ReactNode;
type GuideLogoRenderer = (channel: GuideLayout["channels"][number]) => ReactNode;

interface GuideSurfaceProps {
  density?: Density;
  filter?: GuideFilter;
  filters?: readonly GuideFilterOption[];
  layout: GuideLayout;
  onFilterChange?: (filter: GuideFilter) => void;
  onSelectionChange: (selection: GuideSelection) => void;
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
  GuideExperienceProps,
  GuideFilter,
  GuideFilterOption,
  GuideLogoRenderer,
  GuideSurfaceProps,
  GuideUnavailableState,
};
