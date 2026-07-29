import type { ServicesView } from "@loomarr/api";

interface ServicesPanelProps {
  /** Every configured integration plus Loomarr's own row, from GET /v1/system/services. */
  view: ServicesView;
  /**
   * Route the operator to the settings that own a failing connection. Called with the
   * row's `settingsGroup` — a red dot with nowhere to go is a puzzle, not a diagnosis.
   */
  onFix: (settingsGroup: string) => void;
  /** True while a poll is in flight, so the panel can show it is re-checking. */
  refreshing?: boolean;
  className?: string;
}

export type { ServicesPanelProps };
