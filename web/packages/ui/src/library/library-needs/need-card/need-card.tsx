import { Action, Disclosure, StatusDot, Surface, Text, useWindowWidth } from "@loomarr/design-system";
import { useState } from "react";

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
  const [files, setFiles] = useState(false);
  const wide = useWindowWidth() >= 640;
  const card = (
    <Surface gap="control" padding="section">
      <Line alignItems="flex-start" columnGap="section" flexWrap={wide ? "nowrap" : "wrap"} rowGap="control">
        <Stack flexShrink={0} width={wide ? 144 : 112}>
          <LibraryPoster art={best?.poster} big name={needName(need)} />
        </Stack>
        <Stack flexBasis={240} flexGrow={1} gap="control" minWidth={0}>
          {phase ? (
            <Line paddingVertical="section">
              <StatusDot label="Done" tone="ok" />
              <Text textRole="title">
                {phase === "ignored" ? "Okay, we'll leave it out." : "Added to your library."}
              </Text>
            </Line>
          ) : (
            <>
              <Text textRole="title">
                {best ? `Is this ${nameYear(best)}?` : "We're not sure what this is"}
              </Text>
              <Text textRole="body" tone="secondary">
                {best
                  ? reasonFor(best)
                  : "The file name doesn't say what it is. Find it to pick the title yourself."}
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
                    <Action onPress={() => setOther(!other)} variant="outline">
                      Pick a different title
                    </Action>
                  </>
                ) : (
                  <Action onPress={() => setOther(!other)} variant="outline">
                    Find it
                  </Action>
                )}
                <Action onPress={onIgnore} variant="ghost">
                  Ignore this
                </Action>
              </Line>
              {failed ? <NotSaved /> : null}
              {other ? (
                <PickTitle kind={need.kind} onPick={onPick} onSearch={onSearch} others={need.others} />
              ) : null}
              {need.files.length > 0 ? (
                <Disclosure
                  expanded={files}
                  label={need.files.length === 1 ? "1 file" : `${need.files.length} files`}
                  onExpandedChange={setFiles}
                >
                  <Stack gap={4} padding="control">
                    {need.files.map((f) => (
                      <Text key={f} textRole="compact" tone="secondary">
                        {fileName(f)}
                      </Text>
                    ))}
                  </Stack>
                </Disclosure>
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
