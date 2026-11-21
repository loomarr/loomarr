import { Action, StatusDot, Text } from "@loomarr/design-system";

import { Line, Stack } from "../../library-poster";
import type { LibraryNeed } from "../library-needs.type";
import { LockIn } from "../lock-in";
import { needName } from "../need-name";
import { NeedThumb } from "../need-thumb";
import { NotSaved } from "../not-saved";

const StripRow = ({
  need,
  answered,
  failed,
  onYes,
  onDone,
  onOpenNeeds,
}: {
  need: LibraryNeed;
  answered: boolean;
  /** The last answer was not saved. */
  failed?: boolean;
  onYes: () => void;
  onDone: () => void;
  onOpenNeeds: () => void;
}) => {
  const best = need.best;
  const name = needName(need);
  const what = need.kind === "series" ? "show" : "movie";
  const line = best
    ? `${best.name} — is this the ${best.year ? `${best.year} ` : ""}${what}?`
    : `${name} — we're not sure what this is`;
  const row = (
    <Line columnGap="control" flexWrap="wrap" paddingVertical="control" rowGap="inline">
      <Line flexBasis={240} flexGrow={1} flexWrap="nowrap" minWidth={0}>
        <NeedThumb art={best?.poster} name={name} width={36} />
        <Stack flexShrink={1} minWidth={0}>
          <Text textRole="body">{answered ? `Done — ${name} is sorted.` : line}</Text>
          {failed && !answered ? <NotSaved /> : null}
        </Stack>
      </Line>
      {answered ? (
        <StatusDot label="Done" tone="ok" />
      ) : best ? (
        <Line>
          <Action onPress={onYes} variant="outline">
            Yes, that's it
          </Action>
          <Action onPress={onOpenNeeds} variant="ghost">
            Pick a different title
          </Action>
        </Line>
      ) : (
        <Action onPress={onOpenNeeds} variant="outline">
          Find it
        </Action>
      )}
    </Line>
  );
  return answered ? <LockIn onDone={onDone}>{row}</LockIn> : row;
};

export { StripRow };
