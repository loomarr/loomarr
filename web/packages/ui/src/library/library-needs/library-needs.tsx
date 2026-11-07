import {
  Action,
  Disclosure,
  StatusDot,
  Surface,
  Text,
  useReducedMotionPreference,
  useWindowWidth,
} from "@loomarr/design-system";
import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";
import { View } from "react-native";

import { LibraryPoster, Line, Stack } from "../library-poster";
import { nameYear, PickTitle } from "../pick-title";
import type {
  LibraryNeed,
  LibraryNeedCandidate,
  LibraryNeedSearch,
  LibraryNeedsCardsProps,
  LibraryNeedsStripProps,
} from "./library-needs.type";

// NEEDS YOU (#2151 decision 27, design/library variant D): the titles Loomarr can't tell what they
// are. The strip on the Library page asks about the first four; Needs you holds one card each. The
// words are the mock's word sheet: "Yes, that's it", "Pick a different title", "Ignore this".

const STRIP_FIRST = 4;
// An answer locks the card in (static → clear) and it leaves once that has been seen.
const LOCK_CLEAR_MS = 200;
const LOCK_HOLD_MS = 1400;

const fileName = (path: string) => path.split("/").filter(Boolean).pop() ?? path;

// The one reason line: which of the file's facts the guess agrees with.
const reasonFor = (c: LibraryNeedCandidate): string => {
  if (c.nameMatches && c.lengthMatches && !c.yearMatches)
    return "The name and length match; the year doesn't.";
  if (c.nameMatches && c.yearMatches) return "The name and year match.";
  return "It looks close, but we can't be sure.";
};

// What a need is called where it is named: its best guess, else its first file.
const needName = (need: LibraryNeed) => need.best?.name ?? fileName(need.files[0] ?? "");

// A poster small enough for a row: the tile alone.
const Thumb = ({ name, art, width }: { name: string; art?: ReactNode; width: number }) => (
  <Stack flexShrink={0} width={width}>
    <LibraryPoster art={art} name={art ? name : ""} />
  </Stack>
);

// The answered card, held a moment: it starts greyed and clears, then leaves. The clearing is a
// CSS transition (react-native-web passes it through); Animated needs a global the web build lacks.
const LockIn = ({ children, onDone }: { children: ReactNode; onDone: () => void }) => {
  const reduced = useReducedMotionPreference();
  const [clear, setClear] = useState(false);
  // The latest onDone, so a re-render (a refetch) doesn't restart the hold.
  const done = useRef(onDone);
  done.current = onDone;
  useEffect(() => {
    const start = setTimeout(() => setClear(true), 30);
    const leave = setTimeout(() => done.current(), LOCK_HOLD_MS);
    return () => {
      clearTimeout(start);
      clearTimeout(leave);
    };
  }, []);
  const transition =
    reduced === false ? { transitionDuration: `${LOCK_CLEAR_MS}ms`, transitionProperty: "opacity" } : {};
  return <View style={[{ opacity: clear ? 1 : 0.6 }, transition as object]}>{children}</View>;
};

// The answers given here. An answered title locks in and leaves on its own time: the list that
// refetches after the answer no longer holds it, so it is kept where it was until it has left. A
// failed answer puts the question back.
type Phase = "added" | "ignored" | "gone";
const useAnswers = (needs: readonly LibraryNeed[]) => {
  const [phases, setPhases] = useState<Record<string, Phase>>({});
  const last = useRef<readonly LibraryNeed[]>([]);
  const fresh = new Map(needs.map((n) => [n.id, n]));
  const kept: LibraryNeed[] = [];
  for (const n of last.current) {
    const now = fresh.get(n.id);
    if (now) kept.push(now);
    else if (phases[n.id] === "added" || phases[n.id] === "ignored") kept.push(n);
  }
  const seen = new Set(kept.map((n) => n.id));
  const list = [...kept, ...needs.filter((n) => !seen.has(n.id))].filter((n) => phases[n.id] !== "gone");
  last.current = list;
  const set = useCallback((id: string, phase?: Phase) => {
    setPhases((all) => {
      const next = { ...all };
      if (phase) next[id] = phase;
      else delete next[id];
      return next;
    });
  }, []);
  const answer = (id: string, phase: "added" | "ignored", save: Promise<void>) => {
    set(id, phase);
    save.catch(() => set(id));
  };
  return { list, phases, answer, done: (id: string) => set(id, "gone") };
};

// ── The strip ─────────────────────────────────────────────────────────────────────────────────

const StripRow = ({
  need,
  answered,
  onYes,
  onDone,
  onOpenNeeds,
}: {
  need: LibraryNeed;
  answered: boolean;
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
        <Thumb art={best?.poster} name={name} width={36} />
        <Stack flexShrink={1} minWidth={0}>
          <Text textRole="body">{answered ? `Done — ${name} is sorted.` : line}</Text>
        </Stack>
      </Line>
      {answered ? (
        <StatusDot label="Done" tone="ok" />
      ) : best ? (
        <Line>
          <Action onPress={onYes} variant="secondary">
            Yes, that's it
          </Action>
          <Action onPress={onOpenNeeds} variant="ghost">
            Pick a different title
          </Action>
        </Line>
      ) : (
        <Action onPress={onOpenNeeds} variant="secondary">
          Find it
        </Action>
      )}
    </Line>
  );
  return answered ? <LockIn onDone={onDone}>{row}</LockIn> : row;
};

const LibraryNeedsStrip = ({ needs, onLink, onOpenNeeds }: LibraryNeedsStripProps) => {
  const { list: open, phases, answer, done } = useAnswers(needs);
  const [all, setAll] = useState(false);
  if (open.length === 0) return null;
  const shown = all ? open : open.slice(0, STRIP_FIRST);
  const waiting = open.filter((n) => !phases[n.id]).length;
  return (
    <Surface aria-label="Needs you" borderColor="stateWarning" gap="inline" padding="control" role="region">
      <Text textRole="headline">{`Needs you (${waiting})`}</Text>
      <Stack>
        {shown.map((need, i) => (
          <Stack borderColor="borderDecorative" borderTopWidth={i === 0 ? 0 : 1} key={need.id}>
            <StripRow
              answered={phases[need.id] !== undefined}
              need={need}
              onDone={() => done(need.id)}
              onOpenNeeds={onOpenNeeds}
              onYes={() => {
                if (need.best) answer(need.id, "added", onLink(need.id, need.best.tmdbId));
              }}
            />
          </Stack>
        ))}
      </Stack>
      {open.length > STRIP_FIRST && !all ? (
        <Line>
          <Action onPress={() => setAll(true)} variant="ghost">
            {`Show ${open.length - STRIP_FIRST} more`}
          </Action>
        </Line>
      ) : null}
    </Surface>
  );
};

// ── Needs you: one card each ──────────────────────────────────────────────────────────────────

const NeedCard = ({
  need,
  primary,
  phase,
  onYes,
  onPick,
  onIgnore,
  onDone,
  onSearch,
}: {
  need: LibraryNeed;
  primary: boolean;
  phase?: Phase;
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
                      variant={primary ? undefined : "secondary"}
                    >
                      Yes, that's it
                    </Action>
                    <Action onPress={() => setOther(!other)} variant="secondary">
                      Pick a different title
                    </Action>
                  </>
                ) : (
                  <Action onPress={() => setOther(!other)} variant="secondary">
                    Find it
                  </Action>
                )}
                <Action onPress={onIgnore} variant="ghost">
                  Ignore this
                </Action>
              </Line>
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

const LibraryNeedsCards = ({ needs, onLink, onIgnore, onSearch }: LibraryNeedsCardsProps) => {
  const { list: open, phases, answer, done } = useAnswers(needs);
  return (
    <Surface aria-label="Needs you" backgroundColor="transparent" borderWidth={0} gap="control" role="region">
      <Text as="h2" textRole="headline">{`Needs you (${open.filter((n) => !phases[n.id]).length})`}</Text>
      {open.length === 0 ? (
        <Text textRole="body" tone="secondary">
          Nothing needs you.
        </Text>
      ) : null}
      {open.map((need, i) => (
        <NeedCard
          key={need.id}
          need={need}
          onDone={() => done(need.id)}
          onIgnore={() => answer(need.id, "ignored", onIgnore(need.id))}
          onPick={(c) => answer(need.id, "added", onLink(need.id, c.tmdbId))}
          onSearch={onSearch}
          onYes={() => {
            if (need.best) answer(need.id, "added", onLink(need.id, need.best.tmdbId));
          }}
          phase={phases[need.id]}
          primary={i === 0}
        />
      ))}
    </Surface>
  );
};

export { LibraryNeedsCards, LibraryNeedsStrip };
