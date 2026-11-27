import { Notice, Surface } from "@loomarr/design-system";
import { dayOffsetOf, windowName } from "../guide-page-model";

interface GuideRefreshNoticeProps {
  anchorMs: number;
  dayOffset: number;
  /** The window the viewer asked for. */
  from: number;
  isFetching: boolean;
  onRetry: () => void;
  /** The earlier window still on screen, when the requested one failed to load. */
  showing?: { fromMs: number; toMs: number };
  timeZone?: string;
  to: number;
}

// A Guide that failed to refresh while an earlier one is still drawn: the notice says which window
// is on screen and offers the retry.
const GuideRefreshNotice = ({
  anchorMs,
  dayOffset,
  from,
  isFetching,
  onRetry,
  showing,
  timeZone,
  to,
}: GuideRefreshNoticeProps) => (
  // The same inset the TV and phone Guide give it (GuideJourney).
  <Surface backgroundColor="transparent" borderWidth={0} padding="control">
    <Notice
      // Mounted while the retry runs, so the focus on its button stays; a press during it is the
      // same retry.
      action={{
        label: isFetching ? "Trying again…" : "Try again",
        onPress: () => {
          if (!isFetching) onRetry();
        },
      }}
      body={
        showing
          ? // The controls stay on what the viewer chose (Try again reads it); the notice names it
            // and the grid still drawn, so neither is read as the other.
            `${windowName(dayOffset, from, to, anchorMs, timeZone)} didn't load. Showing ${windowName(
              dayOffsetOf(showing.fromMs, timeZone),
              showing.fromMs,
              showing.toMs,
              anchorMs,
              timeZone,
            )}, the last one that did.`
          : "Showing what Loomarr had a moment ago."
      }
      title="Couldn't refresh the Guide"
      tone="warn"
    />
  </Surface>
);

export { GuideRefreshNotice };
