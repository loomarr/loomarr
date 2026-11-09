import { formatBytes } from "@loomarr/core/format";
import {
  Action,
  Badge,
  Disclosure,
  RadioGroup,
  RadioItem,
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
  LibraryNeedsAnswers,
  LibraryNeedsCardsProps,
  LibraryNeedsStripProps,
  LibraryOrderAnswer,
  LibraryOrderChoice,
  LibraryOrderFile,
  LibraryOrderKind,
  LibraryOrderNeed,
  LibraryProblem,
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
  if (c.nameMatches && c.episodesMatches)
    return c.yearMatches ? "The name, year and episodes match." : "The name and episodes match.";
  if (c.nameMatches && c.yearMatches) return "The name and year match.";
  return "It looks close, but we can't be sure.";
};

// A title that needs a look, as Needs you words it (decision 24, the mock's word sheet): the strip's
// line, the card's headline and its one reason. A title with both kinds of problem is two entries;
// See why opens its page.
interface ProblemEntry {
  key: string;
  id: string;
  name: string;
  poster?: ReactNode;
  line: string;
  headline: string;
  reason: string;
}

const problemEntries = (problems: readonly LibraryProblem[]): ProblemEntry[] =>
  problems.flatMap((p) => {
    const out: ProblemEntry[] = [];
    const base = { id: p.id, name: p.name, poster: p.poster };
    if (p.unplayable > 0) {
      const movie = p.kind === "movie";
      const one = movie || p.unplayable === 1;
      const episodes = p.unplayable === 1 ? "1 episode" : `${p.unplayable} episodes`;
      out.push({
        ...base,
        key: `${p.id}:unplayable`,
        line: movie ? `${p.name} — can't be played` : `${p.name} — ${episodes} can't be played`,
        headline: movie
          ? `${p.name} can't be played`
          : p.unplayable === 1
            ? `${p.name} has an episode that can't be played`
            : `${p.name} has ${episodes} that can't be played`,
        reason: one
          ? "The file may be damaged. Loomarr skips it until it's fixed."
          : "The files may be damaged. Loomarr skips them until they're fixed.",
      });
    }
    if (p.extraBytes > 0) {
      const size = formatBytes(p.extraBytes);
      out.push({
        ...base,
        key: `${p.id}:extra`,
        line: `${p.name} — an extra copy is taking ${size}`,
        headline: `${p.name} has an extra copy`,
        reason: `The extra copy is taking ${size} and never plays. Removing it gets the space back.`,
      });
    }
    return out;
  });

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
// failed answer puts the question back and says so, as the title page does.
type Phase = "added" | "ignored" | "gone";
const useAnswers = <T extends { id: string }>(needs: readonly T[]) => {
  const [phases, setPhases] = useState<Record<string, Phase>>({});
  const [failed, setFailed] = useState<Record<string, boolean>>({});
  const last = useRef<readonly T[]>([]);
  const fresh = new Map(needs.map((n) => [n.id, n]));
  const kept: T[] = [];
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
    setFailed((all) => ({ ...all, [id]: false }));
    save.catch(() => {
      set(id);
      setFailed((all) => ({ ...all, [id]: true }));
    });
  };
  return { list, phases, failed, answer, done: (id: string) => set(id, "gone") };
};

// The line a card shows under its actions when its answer was not saved.
const NotSaved = () => (
  <Text textRole="body" tone="secondary">
    Couldn't save your answer. Try again in a moment.
  </Text>
);

// ── Episode order (#2151 decision 53, the approved mock in project/prototypes) ──────────────────
// TMDB lists some shows in more than one order (its episode groups). Each is named in plain words
// by its kind; TMDB's own name is kept for a kind with no plain name, or when two share one.

const PLAIN_ORDER: Partial<Record<LibraryOrderKind, string>> = {
  absolute: "One long run",
  dvd: "DVD order",
  digital: "Digital order",
  story: "Story order",
  production: "Production order",
  tv: "TV order",
};

const FILES = "files";
const choiceKey = (c: LibraryOrderChoice) =>
  c.choice === "catalog" ? "catalog" : `order:${c.orderId ?? ""}`;

const plural = (n: number, one: string) => `${n} ${one}${n === 1 ? "" : "s"}`;

interface OrderOption {
  key: string;
  answer: LibraryOrderAnswer;
  name: string;
  sub: string;
  fits: boolean;
  /** How the done line says it: "Harbor Lights {done}." */
  done: string;
}

const orderOptions = (need: LibraryOrderNeed): OrderOption[] => {
  const kinds = need.choices.map((c) => c.kind);
  const options = need.choices.map((c): OrderOption => {
    const shared = kinds.filter((k) => k === c.kind).length > 1;
    const plain = c.kind && !shared ? PLAIN_ORDER[c.kind] : undefined;
    const seasons = c.seasons.filter((s) => s.season > 0 && s.episodes > 0).length;
    const episodes = plural(c.episodes, "episode");
    const sub =
      c.kind === "absolute"
        ? `${episodes}, no seasons`
        : seasons > 0
          ? `${plural(seasons, "season")} · ${episodes}`
          : episodes;
    if (c.choice === "catalog")
      return {
        key: choiceKey(c),
        answer: { choice: "catalog" },
        name: "As first aired",
        sub,
        fits: c.fits,
        done: "uses the order it first aired in",
      };
    const name = plain ?? c.name ?? "Another order";
    return {
      key: choiceKey(c),
      answer: { choice: "order", orderId: c.orderId ?? "" },
      name,
      sub,
      fits: c.fits,
      done: c.kind === "absolute" && plain ? "plays as one long run" : `uses ${name}`,
    };
  });
  options.push({
    key: FILES,
    answer: { choice: "files" },
    name: "Keep the file names",
    sub: "Each episode keeps the name and number in its file. We won't ask again.",
    fits: false,
    done: "keeps the names in its files",
  });
  return options;
};

// One order besides the files' own: nothing to choose, only that a season doesn't line up.
const onlyCatalog = (need: LibraryOrderNeed) => need.choices.length <= 1;

// The first season on disk that doesn't end where TMDB's does, said in a line.
const mismatchLine = (need: LibraryOrderNeed): string => {
  const waiting = need.files.length;
  const sizes = new Map((need.choices[0]?.seasons ?? []).map((s) => [s.season, s.episodes]));
  const off = need.disk.find((d) => sizes.get(d.season) !== d.episodes);
  const one = waiting === 1;
  const still = one
    ? "It still plays, named as it is in its file."
    : `Those ${waiting} still play, named as they are in their files.`;
  if (!off)
    return one
      ? "1 episode still plays, named as it is in its file."
      : `${waiting} episodes still play, named as they are in their files.`;
  const listed = sizes.get(off.season) ?? 0;
  return listed === 0
    ? `Season ${off.season} on your disk isn't one TMDB lists. ${still}`
    : `Season ${off.season} on your disk has ${plural(off.episodes, "episode")}; TMDB lists ${listed}. ${still}`;
};

const FILES_SHOWN = 3;
const pad = (n: number) => String(n).padStart(2, "0");

const OrderFiles = ({ files }: { files: readonly LibraryOrderFile[] }) => {
  const [open, setOpen] = useState(false);
  if (files.length === 0) return null;
  return (
    <Disclosure expanded={open} label={plural(files.length, "episode")} onExpandedChange={setOpen}>
      <Stack gap={4} padding="control">
        {files.slice(0, FILES_SHOWN).map((f) => (
          <Text key={f.path} textRole="compact" tone="secondary">
            {`S${pad(f.season)}E${pad(f.episode)} — ${fileName(f.path)}`}
          </Text>
        ))}
        {files.length > FILES_SHOWN ? (
          <Text textRole="hint" tone="muted">{`and ${files.length - FILES_SHOWN} more`}</Text>
        ) : null}
      </Stack>
    </Disclosure>
  );
};

const OrderRadio = ({ option }: { option: OrderOption }) => (
  <RadioItem label={option.name} ringRadius={0} value={option.key}>
    {({ checked }) => (
      <Line
        alignItems="flex-start"
        backgroundColor={checked ? "surfaceFocus" : "transparent"}
        borderRadius={0}
        columnGap="control"
        flexWrap="nowrap"
        paddingHorizontal={14}
        paddingVertical={12}
      >
        <Stack
          alignItems="center"
          borderColor={checked ? "actionPrimary" : "borderControl"}
          borderRadius="round"
          borderWidth={1.5}
          flexShrink={0}
          height={18}
          justifyContent="center"
          marginTop={1}
          width={18}
        >
          {checked ? (
            <Stack backgroundColor="actionPrimary" borderRadius="round" height={8} width={8} />
          ) : null}
        </Stack>
        <Stack flexShrink={1} gap={2} minWidth={0}>
          <Line columnGap="control" flexWrap="wrap" rowGap={4}>
            <Text strong textRole="body">
              {option.name}
            </Text>
            {option.fits ? <Badge tone="success">Fits your files</Badge> : null}
          </Line>
          <Text textRole="compact" tone="muted">
            {option.sub}
          </Text>
        </Stack>
      </Line>
    )}
  </RadioItem>
);

const orderLine = (need: LibraryOrderNeed) =>
  onlyCatalog(need)
    ? `${need.name} — ${need.files.length === 1 ? "1 episode keeps its file name" : `${need.files.length} episodes keep their file names`}`
    : `${need.name} — which order are the episodes in?`;

// The strip asks in one line; Choose opens Needs you, since the question has more than two answers.
const OrderRow = ({ need, onOpenNeeds }: { need: LibraryOrderNeed; onOpenNeeds: () => void }) => (
  <Line columnGap="control" flexWrap="wrap" paddingVertical="control" rowGap="inline">
    <Line flexBasis={240} flexGrow={1} flexWrap="nowrap" minWidth={0}>
      <Thumb art={need.poster} name={need.name} width={36} />
      <Stack flexShrink={1} minWidth={0}>
        <Text textRole="body">{orderLine(need)}</Text>
      </Stack>
    </Line>
    <Action onPress={onOpenNeeds} variant="secondary">
      {onlyCatalog(need) ? "See why" : "Choose"}
    </Action>
  </Line>
);

const OrderCard = ({
  need,
  primary,
  done,
  failed,
  onChoose,
  onDone,
  onOpenTitle,
}: {
  need: LibraryOrderNeed;
  primary: boolean;
  /** The answer given, as the done line says it; absent while it is asked. */
  done?: { line: string; fromTmdb: boolean };
  /** The last answer was not saved. */
  failed?: boolean;
  onChoose: (option: OrderOption) => void;
  onDone: () => void;
  onOpenTitle: (id: string) => void;
}) => {
  const options = orderOptions(need);
  const [picked, setPicked] = useState(() => (options.find((o) => o.fits) ?? options[0])?.key ?? FILES);
  const wide = useWindowWidth() >= 640;
  const waiting = need.files.length;
  const chosen = options.find((o) => o.key === picked);
  const tone = primary ? { tone: "primary" as const } : { variant: "secondary" as const };
  const files = options.find((o) => o.key === FILES);
  const card = (
    <Surface gap="control" padding="section">
      <Line alignItems="flex-start" columnGap="section" flexWrap={wide ? "nowrap" : "wrap"} rowGap="control">
        <Stack flexShrink={0} width={wide ? 144 : 112}>
          <LibraryPoster art={need.poster} big name={need.name} />
        </Stack>
        <Stack flexBasis={240} flexGrow={1} gap="control" minWidth={0}>
          {done ? (
            <>
              <Line paddingTop="section">
                <StatusDot label="Done" tone="ok" />
                <Text textRole="title">{`Done — ${need.name} ${done.line}.`}</Text>
              </Line>
              {done.fromTmdb ? (
                <Text textRole="compact" tone="muted">
                  {`${plural(waiting, "episode")} now ${waiting === 1 ? "has its" : "have their"} names and details from TMDB.`}
                </Text>
              ) : null}
            </>
          ) : onlyCatalog(need) ? (
            <>
              <Text textRole="title">{`Some ${need.name} episodes don't line up with TMDB`}</Text>
              <Text textRole="body" tone="secondary">
                {mismatchLine(need)}
              </Text>
              <Line>
                <Action onPress={() => files && onChoose(files)} {...tone}>
                  That's fine
                </Action>
                <Action onPress={() => onOpenTitle(need.id)} variant="secondary">
                  See the show
                </Action>
              </Line>
              {failed ? <NotSaved /> : null}
              <OrderFiles files={need.files} />
            </>
          ) : (
            <>
              <Text textRole="title">{`Which order are your ${need.name} episodes in?`}</Text>
              <Text textRole="body" tone="secondary">
                {`TMDB lists this show in more than one order, and your files don't say which they follow. Until you choose, ${plural(waiting, "episode")} use the name and number in ${waiting === 1 ? "its" : "their"} file.`}
              </Text>
              <RadioGroup
                borderColor="borderDecorative"
                borderRadius="control"
                borderWidth={1}
                label="Episode order"
                onValueChange={setPicked}
                overflow="hidden"
                values={options.map((o) => o.key)}
                value={picked}
              >
                {options.map((o, i) => (
                  <Stack
                    borderColor="borderDecorative"
                    borderRadius={0}
                    borderTopWidth={i === 0 ? 0 : 1}
                    key={o.key}
                  >
                    <OrderRadio option={o} />
                  </Stack>
                ))}
              </RadioGroup>
              <Line>
                <Action onPress={() => chosen && onChoose(chosen)} {...tone}>
                  {picked === FILES ? "Keep the file names" : "Use this order"}
                </Action>
              </Line>
              {failed ? <NotSaved /> : null}
              <OrderFiles files={need.files} />
            </>
          )}
        </Stack>
      </Line>
    </Surface>
  );
  return done ? <LockIn onDone={onDone}>{card}</LockIn> : card;
};

// The answers given to order needs: the shared lock-in, and what each said for its done line.
const useOrderAnswers = (
  orders: readonly LibraryOrderNeed[],
  onChooseOrder: LibraryNeedsAnswers["onChooseOrder"],
) => {
  const answers = useAnswers(orders);
  const [said, setSaid] = useState<Record<string, OrderOption>>({});
  const choose = (need: LibraryOrderNeed, option: OrderOption) => {
    setSaid((all) => ({ ...all, [need.id]: option }));
    answers.answer(need.id, "added", onChooseOrder(need.id, option.answer));
  };
  const doneOf = (id: string) => {
    const option = said[id];
    return answers.phases[id] && option ? { line: option.done, fromTmdb: option.key !== FILES } : undefined;
  };
  return { ...answers, choose, doneOf };
};

// ── The strip ─────────────────────────────────────────────────────────────────────────────────

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
        <Thumb art={best?.poster} name={name} width={36} />
        <Stack flexShrink={1} minWidth={0}>
          <Text textRole="body">{answered ? `Done — ${name} is sorted.` : line}</Text>
          {failed && !answered ? <NotSaved /> : null}
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

const ProblemRow = ({ entry, onOpenTitle }: { entry: ProblemEntry; onOpenTitle: (id: string) => void }) => (
  <Line columnGap="control" flexWrap="wrap" paddingVertical="control" rowGap="inline">
    <Line flexBasis={240} flexGrow={1} flexWrap="nowrap" minWidth={0}>
      <Thumb art={entry.poster} name={entry.name} width={36} />
      <Stack flexShrink={1} minWidth={0}>
        <Text textRole="body">{entry.line}</Text>
      </Stack>
    </Line>
    <Action onPress={() => onOpenTitle(entry.id)} variant="secondary">
      See why
    </Action>
  </Line>
);

const LibraryNeedsStrip = ({
  needs,
  orders,
  problems,
  onLink,
  onOpenNeeds,
  onOpenTitle,
}: LibraryNeedsStripProps) => {
  const { list: open, phases, failed, answer, done } = useAnswers(needs);
  const [all, setAll] = useState(false);
  const entries = problemEntries(problems);
  // The shows whose order to say, the titles to answer, then the ones that need a look, as the
  // mocks list them.
  const rows: { key: string; row: ReactNode }[] = [
    ...orders.map((need) => ({
      key: `order:${need.id}`,
      row: <OrderRow need={need} onOpenNeeds={onOpenNeeds} />,
    })),
    ...open.map((need) => ({
      key: need.id,
      row: (
        <StripRow
          answered={phases[need.id] !== undefined}
          failed={failed[need.id]}
          need={need}
          onDone={() => done(need.id)}
          onOpenNeeds={onOpenNeeds}
          onYes={() => {
            if (need.best) answer(need.id, "added", onLink(need.id, need.best.externalId));
          }}
        />
      ),
    })),
    ...entries.map((entry) => ({
      key: entry.key,
      row: <ProblemRow entry={entry} onOpenTitle={onOpenTitle} />,
    })),
  ];
  if (rows.length === 0) return null;
  const shown = all ? rows : rows.slice(0, STRIP_FIRST);
  const waiting = orders.length + open.filter((n) => !phases[n.id]).length + entries.length;
  return (
    <Surface aria-label="Needs you" borderColor="stateWarning" gap="inline" padding="control" role="region">
      <Text textRole="headline">{`Needs you (${waiting})`}</Text>
      <Stack>
        {shown.map(({ key, row }, i) => (
          <Stack borderColor="borderDecorative" borderTopWidth={i === 0 ? 0 : 1} key={key}>
            {row}
          </Stack>
        ))}
      </Stack>
      {rows.length > STRIP_FIRST && !all ? (
        <Line>
          <Action onPress={() => setAll(true)} variant="ghost">
            {`Show ${rows.length - STRIP_FIRST} more`}
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
            <Action onPress={() => onOpenTitle(entry.id)} variant="secondary">
              See why
            </Action>
          </Line>
        </Stack>
      </Line>
    </Surface>
  );
};

const LibraryNeedsCards = ({
  needs,
  orders,
  problems,
  onLink,
  onIgnore,
  onChooseOrder,
  onSearch,
  onOpenTitle,
}: LibraryNeedsCardsProps) => {
  const { list: open, phases, failed, answer, done } = useAnswers(needs);
  const shows = useOrderAnswers(orders, onChooseOrder);
  const entries = problemEntries(problems);
  const waiting =
    shows.list.filter((n) => !shows.phases[n.id]).length +
    open.filter((n) => !phases[n.id]).length +
    entries.length;
  return (
    <Surface aria-label="Needs you" backgroundColor="transparent" borderWidth={0} gap="control" role="region">
      <Text as="h2" textRole="headline">{`Needs you (${waiting})`}</Text>
      {shows.list.length + open.length + entries.length === 0 ? (
        <Text textRole="body" tone="secondary">
          Nothing needs you.
        </Text>
      ) : null}
      {shows.list.map((need, i) => (
        <OrderCard
          done={shows.doneOf(need.id)}
          failed={shows.failed[need.id]}
          key={need.id}
          need={need}
          onChoose={(option) => shows.choose(need, option)}
          onDone={() => shows.done(need.id)}
          onOpenTitle={onOpenTitle}
          primary={i === 0}
        />
      ))}
      {open.map((need, i) => (
        <NeedCard
          key={need.id}
          need={need}
          onDone={() => done(need.id)}
          onIgnore={() => answer(need.id, "ignored", onIgnore(need.id))}
          onPick={(c) => answer(need.id, "added", onLink(need.id, c.externalId))}
          onSearch={onSearch}
          onYes={() => {
            if (need.best) answer(need.id, "added", onLink(need.id, need.best.externalId));
          }}
          failed={failed[need.id]}
          phase={phases[need.id]}
          primary={shows.list.length === 0 && i === 0}
        />
      ))}
      {entries.map((entry) => (
        <ProblemCard entry={entry} key={entry.key} onOpenTitle={onOpenTitle} />
      ))}
    </Surface>
  );
};

export { LibraryNeedsCards, LibraryNeedsStrip };
