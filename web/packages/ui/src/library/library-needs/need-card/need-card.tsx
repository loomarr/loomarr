import { Action, FileName, StatusDot, Surface, Text, useWindowWidth } from "@loomarr/design-system";
import { type ComponentRef, useEffect, useRef, useState } from "react";

import { LibraryPoster, Line, Stack } from "../../library-poster";
import { nameYear, PickTitle } from "../../pick-title";
import { fileName } from "../file-name";
import type { LibraryNeed, LibraryNeedCandidate, LibraryNeedSearch } from "../library-needs.type";
import { LockIn } from "../lock-in";
import { needName } from "../need-name";
import { NotSaved } from "../not-saved";
import type { Phase } from "../use-answers";

// The one reason line: which of the file's facts the guess agrees with.
const reasonFor = (c: LibraryNeedCandidate): string => {
  if (c.nameMatches && c.lengthMatches && !c.yearMatches)
    return "The name and length match; the year doesn't.";
  if (c.nameMatches && c.episodesMatches)
    return c.yearMatches ? "The name, year and episodes match." : "The name and episodes match.";
  if (c.nameMatches && c.yearMatches) return "The name and year match.";
  return "It looks close, but we can't be sure.";
};

const NeedCard = ({
  need,
  primary,
  phase,
  failed,
  onYes,
  onPick,
  onIgnore,
  onDone,
  onSearch,
}: {
  need: LibraryNeed;
  primary: boolean;
  phase?: Phase;
  /** The last answer was not saved. */
  failed?: boolean;
  onYes: () => void;
  onPick: (c: LibraryNeedCandidate) => void;
  onIgnore: () => void;
  onDone: () => void;
  onSearch: LibraryNeedSearch;
}) => {
  const best = need.best;
  const [other, setOther] = useState(false);
  const wide = useWindowWidth() >= 640;
  const opener = useRef<ComponentRef<typeof Action>>(null);
  // Set by Cancel and Escape: focus goes back to the button that opened the search.
  const [returning, setReturning] = useState(false);
  useEffect(() => {
    if (!returning) return;
    (opener.current as { focus?: () => void } | null)?.focus?.();
    setReturning(false);
  }, [returning]);
  const cancel = () => {
    setOther(false);
    setReturning(true);
  };
  const [first, ...rest] = need.files;
  const card = (
    <Surface gap="control" padding="section">
      <Line alignItems="flex-start" columnGap="section" flexWrap={wide ? "nowrap" : "wrap"} rowGap="control">
        <Stack flexShrink={0} width={wide ? 144 : 112}>
          <LibraryPoster art={best?.poster} big name={needName(need)} />
        </Stack>
        <Stack flexBasis={240} flexGrow={1} flexShrink={1} gap="control" minWidth={0}>
          {phase ? (
            <Line paddingVertical="section">
              <StatusDot label="Done" tone="ok" />
              <Text textRole="title">
                {phase === "ignored" ? "Okay, we'll leave it out." : "Added to your library."}
              </Text>
            </Line>
          ) : (
            <>
              {first ? <FileName name={fileName(first)} strong /> : null}
              {rest.length > 0 ? (
                <Text textRole="compact" tone="secondary">
                  {`With ${rest.length} more ${rest.length === 1 ? "file" : "files"}`}
                </Text>
              ) : null}
              {best ? <Text textRole="title">{`Is this ${nameYear(best)}?`}</Text> : null}
              <Text textRole="body" tone="secondary">
                {best ? reasonFor(best) : "The file name doesn't say what it is."}
              </Text>
              {best?.overview ? (
                <Text textRole="compact" tone="muted">
                  {best.overview}
                </Text>
              ) : null}
              <Line>
                {best ? (
                  <>
                    <Action
                      onPress={onYes}
                      tone={primary ? "primary" : undefined}
                      variant={primary ? undefined : "outline"}
                    >
                      Yes, that's it
                    </Action>
                    <Action onPress={() => setOther(!other)} ref={opener} variant="outline">
                      Pick a different title
                    </Action>
                  </>
                ) : (
                  <Action onPress={() => setOther(!other)} ref={opener} variant="outline">
                    Find it
                  </Action>
                )}
                <Action onPress={onIgnore} variant="ghost">
                  Ignore this
                </Action>
              </Line>
              {failed ? <NotSaved /> : null}
              {other ? (
                <PickTitle
                  kind={need.kind}
                  onCancel={cancel}
                  onPick={onPick}
                  onSearch={onSearch}
                  others={need.others}
                />
              ) : null}
            </>
          )}
        </Stack>
      </Line>
    </Surface>
  );
  return phase ? <LockIn onDone={onDone}>{card}</LockIn> : card;
};

export { NeedCard };
