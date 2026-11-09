import type { StatusWordKind } from "@loomarr/design-system";
import type { ReactElement, ReactNode } from "react";

/**
 * A router link the overview renders as. Its children are replaced by the overview's own, so every
 * page is a real link the browser can open in a new tab.
 */
type SettingsOverviewLink = ReactElement<Record<string, unknown>>;

interface SettingsOverviewPage {
  id: string;
  label: string;
  /** Set when the page is a part of another listed page: "on Sign-in, devices and sharing". */
  note?: string;
  link: SettingsOverviewLink;
}

interface SettingsOverviewProblem {
  /** The first thing to fix, as one sentence in plain words: "Backups missed 2 nights." */
  text: string;
  /** The page that fixes it, by its label. The link reads "Open Backups". */
  pageLabel: string;
  link: SettingsOverviewLink;
}

interface SettingsOverviewCard {
  id: string;
  title: string;
  /** Shown in place of a problem, when there is none. */
  description: string;
  /** Absent while the checks behind the card are still loading. */
  status?: StatusWordKind;
  problem?: SettingsOverviewProblem;
  /** At least one. A long list shows its first four and a count. */
  pages: SettingsOverviewPage[];
}

interface SettingsOverviewAdvanced {
  label: string;
  description: string;
  link: SettingsOverviewLink;
}

interface SettingsOverviewProps {
  /** The cards this viewer can visit, in their resting order. Cards with a problem move first. */
  cards: SettingsOverviewCard[];
  /** The search field and its results, above the summary line. */
  search: ReactNode;
  /** The one quiet row under the cards. Absent for a viewer who cannot open it. */
  advanced?: SettingsOverviewAdvanced;
}

export type {
  SettingsOverviewAdvanced,
  SettingsOverviewCard,
  SettingsOverviewLink,
  SettingsOverviewPage,
  SettingsOverviewProblem,
  SettingsOverviewProps,
};
