import type {
  GuideViewChannel,
  GuideViewContext,
  GuideViewEvent,
  GuideViewState,
} from "@loomarr/core/guide-view";
import type { ReactNode } from "react";

import type { StripAiring, StripProgramme } from "../film-strip/series-runs";

/** One block of a row, carrying what the row draws. */
type GuideViewBlock = StripProgramme & { title: string };

/** A desktop window, a television, or a phone held upright. */
type GuideViewLayout = "desktop" | "phone" | "television";

/** A channel as the part draws it; `guideViewChannels` makes its programmes. */
interface GuideViewEntry extends GuideViewChannel {
  airings: readonly StripAiring[];
  programmes: readonly GuideViewBlock[];
}

interface GuideViewShell {
  layout: GuideViewLayout;
  /** The docked picture, which keeps playing. */
  picture: ReactNode;
}

interface GuideViewLoading extends GuideViewShell {
  loading: true;
}

interface GuideViewReady<C extends GuideViewEntry> extends GuideViewShell {
  loading?: false;
  /** The model's state; the caller runs `guideViewReduce` over `onEvent` and does what it asks. */
  state: GuideViewState;
  context: GuideViewContext<C>;
  onEvent: (event: GuideViewEvent) => void;
  /** The watched channel's lower third, shown while the header describes what is playing. */
  lowerThird: ReactNode;
  /** Words raised by the model or the screen; they show over the Guide. */
  message?: string | undefined;
  artworkUri?: (airing: StripAiring) => string | undefined;
  /** The name of a time's day, "Tomorrow" or a weekday; undefined for today. */
  dayName?: (ms: number) => string | undefined;
  /** Takes focus onto the cursor and draws its ring from the start, as a Guide opened by a key does. */
  autoFocus?: boolean | undefined;
}

type GuideViewProps<C extends GuideViewEntry> = GuideViewLoading | GuideViewReady<C>;

export type {
  GuideViewBlock,
  GuideViewEntry,
  GuideViewLayout,
  GuideViewLoading,
  GuideViewProps,
  GuideViewReady,
};
