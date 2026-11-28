import { Action, Surface, useWindowWidth } from "@loomarr/design-system";

import { LibraryPoster, Line, Stack } from "../../library-poster";
import type { ProblemEntry } from "../problem-entries";
import { ProblemReasons } from "../problem-reasons";

// A title that needs a look: everything wrong with it, and See why, which opens its page.
const ProblemCard = ({
  entry,
  onOpenTitle,
  onSafeTarget,
}: {
  entry: ProblemEntry;
  onOpenTitle: (id: string) => void;
  /** Hands over where focus goes when another card is answered. */
  onSafeTarget?: (node: unknown) => void;
}) => {
  const wide = useWindowWidth() >= 640;
  return (
    <Surface gap="control" padding="section">
      <Line alignItems="flex-start" columnGap="section" flexWrap={wide ? "nowrap" : "wrap"} rowGap="control">
        <Stack flexShrink={0} width={wide ? 144 : 112}>
          <LibraryPoster art={entry.poster} big id={entry.id} name={entry.name} />
        </Stack>
        <Stack flexBasis={240} flexGrow={1} flexShrink={1} gap="control" minWidth={0}>
          <ProblemReasons lead reasons={entry.reasons} />
          <Line>
            <Action
              accessibilityLabel={`See why: ${entry.name}`}
              density={wide ? "pointer" : "touch"}
              onPress={() => onOpenTitle(entry.id)}
              ref={onSafeTarget}
              variant="outline"
            >
              See why
            </Action>
          </Line>
        </Stack>
      </Line>
    </Surface>
  );
};

export { ProblemCard };
