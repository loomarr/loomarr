import type {
  GuideViewChannel,
  GuideViewContext,
  GuideViewEvent,
  GuideViewState,
} from "@loomarr/core/guide-view";
import type { ReactNode, Ref } from "react";

import type { StripAiring, StripProgramme } from "../film-strip";

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
  /** The Guide's own key handling, for a caller that takes keys from the document. */
  handle?: Ref<GuideViewHandle>;
}

/** A key as the browser sends it. */
interface GuideViewKey {
  altKey: boolean;
  ctrlKey: boolean;
  defaultPrevented: boolean;
  key: string;
  metaKey: boolean;
  preventDefault: () => void;
}

interface GuideViewHandle {
  /** The one handler the Guide's own keys go through. A key already taken is left alone. */
  onKeyDown: (event: GuideViewKey) => void;
}

type GuideViewProps<C extends GuideViewEntry> = GuideViewLoading | GuideViewReady<C>;

export type {
  GuideViewBlock,
  GuideViewEntry,
  GuideViewHandle,
  GuideViewKey,
  GuideViewLayout,
  GuideViewLoading,
  GuideViewProps,
  GuideViewReady,
};
