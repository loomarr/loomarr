import { formatBytes, formatClipDuration, formatDuration } from "@loomarr/core/format";
import {
  Action,
  ArtworkFrame,
  FileName,
  StatusDot,
  Surface,
  Text,
  useWindowWidth,
} from "@loomarr/design-system";
import { type ComponentRef, useEffect, useRef, useState } from "react";

import { LibraryPoster, Line, Stack } from "../../library-poster";
import { nameYear, PickTitle } from "../../pick-title";
import { fileName } from "../file-name";
import type { LibraryNeed, LibraryNeedCandidate, LibraryNeedSearch } from "../library-needs.type";
import { LockIn } from "../lock-in";
import { needName } from "../need-name";
import { NotSaved } from "../not-saved";
import type { ProblemEntry } from "../problem-entries";
import { ProblemReasons } from "../problem-reasons";
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

const HOUR_MS = 3_600_000;

// The first file's length and size on one line: "1h 42m · 1.4 GB", "4m 12s · 88 MB". A length not
// read yet is left out rather than shown as nothing.
const fileFacts = (need: LibraryNeed): string => {
  const size = formatBytes(need.bytes);
  if (need.lengthMillis <= 0) return size;
  const length =
    need.lengthMillis >= HOUR_MS ? formatDuration(need.lengthMillis) : formatClipDuration(need.lengthMillis);
  return `${length} · ${size}`;
};

const NeedCard = ({
  need,
  primary,
  phase,
  failed,
  onYes,
  onPick,
  onIgnore,
  onKeep,
  onDone,
  onSearch,
  problem,
  onOpenTitle,
  onSafeTarget,
}: {
  need: LibraryNeed;
  /** Hands over where focus goes when another card is answered. */
  onSafeTarget?: (node: unknown) => void;
  /** What else is wrong with this title, listed on the same card. */
  problem?: ProblemEntry;
  onOpenTitle: (id: string) => void;
  primary: boolean;
  phase?: Phase;
  /** The last answer was not saved. */
  failed?: boolean;
  onYes: () => void;
  onPick: (c: LibraryNeedCandidate) => void;
  onIgnore: () => void;
  /** Says it's the household's own video; offered only when there is no guess at all. */
  onKeep?: () => void;
  onDone: () => void;
  onSearch: LibraryNeedSearch;
}) => {
  const best = need.best;
  const [other, setOther] = useState(false);
  const wide = useWindowWidth() >= 640;
  // A phone is touched, so its answers get the touch target.
  const density = wide ? "pointer" : "touch";
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
  // Every card's buttons look alike, so each one's accessible name says which file it answers.
  const file = first ? fileName(first) : needName(need);
  const card = (
    <Surface gap="control" padding="section">
      <Line alignItems="flex-start" columnGap="section" flexWrap={wide ? "nowrap" : "wrap"} rowGap="control">
        <Stack flexShrink={0} width={wide ? 192 : 128}>
          {/* A frame of the file, so a file named like the others can be told apart. Until one is
              taken the box stays empty: the file name already leads the card. */}
          <ArtworkFrame borderRadius="control" state="ready" width="100%">
            {need.frame ?? <Stack />}
          </ArtworkFrame>
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
              {first ? (
                <Text textRole="compact" tone="secondary">
                  {fileFacts(need)}
                </Text>
              ) : null}
              {rest.length > 0 ? (
                <Text textRole="compact" tone="secondary">
                  {`With ${rest.length} more ${rest.length === 1 ? "file" : "files"}`}
                </Text>
              ) : null}
              {best ? (
                <Line columnGap="control" flexWrap="nowrap">
                  {/* The guess's poster beside its name, to hold up against the frame. */}
                  <Stack flexShrink={0} width={48}>
                    <LibraryPoster art={best.poster} name="" />
                  </Stack>
                  <Text textRole="title">{`Is this ${nameYear(best)}?`}</Text>
                </Line>
              ) : null}
              <Text textRole="body" tone="secondary">
                {best ? reasonFor(best) : "The file name doesn't say what it is."}
              </Text>
              {best?.overview ? (
                <Text textRole="compact" tone="muted">
                  {best.overview}
                </Text>
              ) : null}
              {problem ? <ProblemReasons reasons={problem.reasons} /> : null}
              <Line>
                {best ? (
                  <>
                    <Action
                      accessibilityLabel={`Yes, that's it: ${file}`}
                      density={density}
                      onPress={onYes}
                      tone={primary ? "primary" : undefined}
                      variant={primary ? undefined : "outline"}
                    >
                      Yes, that's it
                    </Action>
                    <Action
                      accessibilityLabel={`Pick a different title: ${file}`}
                      density={density}
                      onPress={() => setOther(!other)}
                      ref={opener}
                      variant="outline"
                    >
                      Pick a different title
                    </Action>
                  </>
                ) : (
                  <Action
                    accessibilityLabel={`Find it: ${file}`}
                    density={density}
                    onPress={() => setOther(!other)}
                    ref={opener}
                    variant="outline"
                  >
                    Find it
                  </Action>
                )}
                {onKeep && !best && need.others.length === 0 ? (
                  <Action
                    accessibilityLabel={`It's ours, keep it as is: ${file}`}
                    density={density}
                    onPress={onKeep}
                    variant="ghost"
                  >
                    It's ours, keep it as is
                  </Action>
                ) : null}
                <Action
                  accessibilityLabel={`Ignore this: ${file}`}
                  density={density}
                  onPress={onIgnore}
                  // Focus lands here, not on Yes: a stray second Enter then only ignores, which Undo reverses.
                  ref={onSafeTarget}
                  variant="ghost"
                >
                  Ignore this
                </Action>
                {problem ? (
                  <Action
                    accessibilityLabel={`See why: ${file}`}
                    density={density}
                    onPress={() => onOpenTitle(need.id)}
                    variant="ghost"
                  >
                    See why
                  </Action>
                ) : null}
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
