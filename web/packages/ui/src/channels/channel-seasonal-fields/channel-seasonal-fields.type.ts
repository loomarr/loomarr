import type { Density } from "@loomarr/design-system";
import type { ReactNode } from "react";

// The three choices each collapse the wire's `"" = default` sentinel and its explicit twin (`auto`,
// `featured`, `loop`) into one value: storage provenance is not a user decision, so the app wiring
// maps the policy onto these and back.
type SeasonalMode = "default" | "exclusive" | "off";
type SeasonalStrength = "default" | "heavy" | "light";
type SeasonalOffSeason = "dark" | "default";

type SeasonalHoliday = { id: string; label: string };

interface ChannelSeasonalFieldsProps {
  density?: Density;
  mode: SeasonalMode;
  onModeChange: (mode: SeasonalMode) => void;
  /** Where the mode's value comes from, with its reset: the app's policy-field indicator. */
  modeSource?: ReactNode;
  /** The holidays the backend knows, in its order. */
  holidays: readonly SeasonalHoliday[];
  /** The ids that count as included. Every id when the channel uses the whole built-in list. */
  includedHolidayIds: readonly string[];
  /** The channel stores no subset, so every built-in holiday is included. */
  allHolidaysIncluded: boolean;
  onHolidayToggle: (id: string, included: boolean) => void;
  strength: SeasonalStrength;
  onStrengthChange: (strength: SeasonalStrength) => void;
  /** What the chosen strength does with this lineup, from the backend's count; absent while it loads. */
  strengthConsequence?: string;
  favourEvenings: boolean;
  onFavourEveningsChange: (favour: boolean) => void;
  offSeason: SeasonalOffSeason;
  onOffSeasonChange: (offSeason: SeasonalOffSeason) => void;
}

export type {
  ChannelSeasonalFieldsProps,
  SeasonalHoliday,
  SeasonalMode,
  SeasonalOffSeason,
  SeasonalStrength,
};
