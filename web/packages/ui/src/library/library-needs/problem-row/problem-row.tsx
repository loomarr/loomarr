import { Action, Text } from "@loomarr/design-system";

import { Line, Stack } from "../../library-poster";
import { NeedThumb } from "../need-thumb";
import type { ProblemEntry } from "../problem-entries";

const ProblemRow = ({ entry, onOpenTitle }: { entry: ProblemEntry; onOpenTitle: (id: string) => void }) => (
  <Line columnGap="control" flexWrap="wrap" paddingVertical="control" rowGap="inline">
    <Line flexBasis={240} flexGrow={1} flexWrap="nowrap" minWidth={0}>
      <NeedThumb art={entry.poster} name={entry.name} width={36} />
      <Stack flexShrink={1} minWidth={0}>
        <Text textRole="body">{entry.line}</Text>
      </Stack>
    </Line>
    <Action onPress={() => onOpenTitle(entry.id)} variant="outline">
      See why
    </Action>
  </Line>
);

export { ProblemRow };
