import { ProgressTrack, StatusDot, Text } from "@loomarr/design-system";

import { Line, Stack, Twirl } from "../../library-poster";
import { scanHeadline } from "../../library-words";
import type { LibraryReadyTitle, LibraryScan } from "../library-settings.type";
import { ReadySoFar } from "../ready-so-far";

// The one line at the top of the status while nothing needs the maintainer.
const Headline = ({
  scan,
  readySoFar = [],
  notAllThere = false,
}: {
  scan: LibraryScan;
  readySoFar?: readonly LibraryReadyTitle[];
  /** A folder in use held nothing, so idle says "Up to date", never "All caught up". */
  notAllThere?: boolean;
}) => {
  const words = scanHeadline(scan, notAllThere);
  if (words.quiet) {
    // An hourly pass with nothing to add: one small line, not the block.
    return (
      <Line gap="control">
        <Twirl />
        <Text textRole="body" tone="secondary">
          {words.text}
        </Text>
      </Line>
    );
  }
  const reading = scan.step === "reading";
  return (
    <>
      <Line gap="control">
        {/* A step moving twirls; paused, done or off keeps the still dot. */}
        {words.moving ? <Twirl /> : <StatusDot label={words.text} tone={words.tone} />}
        <Text textRole="headline">{words.text}</Text>
      </Line>
      {words.running ? (
        <Stack gap="control">
          {/* Adding media is preparation (`tune`); paused drops to the neutral fill. */}
          <ProgressTrack
            height={6}
            label={`Adding your media, ${words.percent} percent`}
            percent={words.percent}
            tone={scan.paused ? "elapsed" : "tune"}
          />
          <Text textRole="body" tone="secondary">
            {words.stepLine}
          </Text>
          {scan.paused ? (
            <Text textRole="body" tone="secondary">
              Picks up again when everyone's done watching.
            </Text>
          ) : (
            <>
              <Text textRole="body" tone="secondary">
                {words.timeLine}
              </Text>
              {words.current ? (
                // A clipped, min-width-0 box is what lets the design system truncate.
                <Stack flexShrink={1} minWidth={0} overflow="hidden">
                  <Text numberOfLines={1} textRole="mono" tone="secondary">
                    {words.current}
                  </Text>
                </Stack>
              ) : null}
              {reading ? (
                <Text textRole="body" tone="secondary">
                  Titles appear as they're ready.
                </Text>
              ) : null}
              {reading && readySoFar.length > 0 ? <ReadySoFar titles={readySoFar} /> : null}
            </>
          )}
        </Stack>
      ) : null}
    </>
  );
};

export { Headline };
