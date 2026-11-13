import { Action, Surface, Text, useWindowWidth } from "@loomarr/design-system";

import { LibraryPoster, Line, Stack } from "../../library-poster";
import type { ProblemEntry } from "../problem-entries";

// A title that needs a look: the headline, its one reason, and See why, which opens its page.
const ProblemCard = ({ entry, onOpenTitle }: { entry: ProblemEntry; onOpenTitle: (id: string) => void }) => {
  const wide = useWindowWidth() >= 640;
  return (
    <Surface gap="control" padding="section">
      <Line alignItems="flex-start" columnGap="section" flexWrap={wide ? "nowrap" : "wrap"} rowGap="control">
        <Stack flexShrink={0} width={wide ? 144 : 112}>
          <LibraryPoster art={entry.poster} big name={entry.name} />
        </Stack>
        <Stack flexBasis={240} flexGrow={1} gap="control" minWidth={0}>
          <Text textRole="title">{entry.headline}</Text>
          <Text textRole="body" tone="secondary">
            {entry.reason}
          </Text>
          <Line>
            <Action onPress={() => onOpenTitle(entry.id)} variant="outline">
              See why
            </Action>
          </Line>
        </Stack>
      </Line>
    </Surface>
  );
};

export { ProblemCard };
