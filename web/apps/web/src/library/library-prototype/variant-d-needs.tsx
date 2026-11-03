// PROTOTYPE — throwaway (#2151 step 2). Variant D · "Needs you": the strip on the Library page and
// the same list expanded into full cards on the Unmatched screen. Plain words per PLAIN-WORDS.md.
import { Action, Disclosure, Field, StatusDot, Surface, Text } from "@loomarr/design-system";
import { useCallback, useState } from "react";
import { type Candidate, itemsFor, UNMATCHED, type UnmatchedGroup } from "./prototype-data";
import type { ScanState, Screen } from "./prototype-state";
import { LockIn, Poster, Thumb } from "./variant-d-parts";

interface Need {
  id: string;
  name: string; // what the poster initial and aria labels use
  line: string; // the strip sentence
  kind: "match" | "problem" | "unknown";
  group?: UnmatchedGroup;
  headline?: string; // problems: the card title
  reason?: string; // problems: the one plain reason line
  opens?: Screen; // problems: where "See why" goes
}

const reasonFor = (c: Candidate): string => {
  const { title, year, runtime } = c.agreement;
  if (title && runtime && !year) return "The name and length match; the year doesn't.";
  if (title && year) return "The name and year match.";
  return "It looks close, but we can't be sure.";
};

const needsFor = (scan: ScanState): Need[] => {
  if (scan === "fresh") return [];
  const out: Need[] = UNMATCHED.map((g) => {
    if (!g.best) {
      return {
        id: g.id,
        name: g.files[0] ?? "Unknown file",
        line: `${g.files[0]} — we're not sure what this is`,
        kind: "unknown" as const,
        group: g,
      };
    }
    const what = g.best.kind === "Series" ? "show" : "movie";
    return {
      id: g.id,
      name: g.best.title,
      line: `${g.best.title} — is this the ${g.best.year} ${what}?`,
      kind: "match" as const,
      group: g,
    };
  });
  for (const it of itemsFor(scan)) {
    for (const p of it.problems ?? []) {
      const stray = /extra copy is taking (\d[\d.]* GB)/.exec(p);
      out.push({
        id: `${it.id}-${p}`,
        name: it.title,
        line: stray
          ? `${it.title} — an extra copy is taking ${stray[1]}`
          : `${it.title} — 1 episode can't be played`,
        kind: "problem",
        headline: stray ? `${it.title} has an extra copy` : `${it.title} has an episode that can't be played`,
        reason: stray
          ? `The extra copy is taking ${stray[1]} and never plays. Removing it gets the space back.`
          : "The file may be damaged. Loomarr skips it until it's fixed.",
        opens: stray ? "item-movie" : "item-series",
      });
    }
  }
  return out;
};

// One number for everything that says how many things need the household.
const needsYouCount = (scan: ScanState): number => needsFor(scan).length;

type Answer = "locked" | "gone";

const useAnswers = () => {
  const [answers, setAnswers] = useState<Record<string, Answer>>({});
  const lock = (id: string) => setAnswers((a) => ({ ...a, [id]: "locked" }));
  const done = useCallback((id: string) => setAnswers((a) => ({ ...a, [id]: "gone" })), []);
  return { answers, lock, done };
};

// ── The strip ─────────────────────────────────────────────────────────────────────────────────
const StripRow = ({
  need,
  locked,
  onYes,
  onDone,
  go,
}: {
  need: Need;
  locked: boolean;
  onYes: () => void;
  onDone: () => void;
  go: (s: Screen) => void;
}) => {
  const body = (
    <div className="flex flex-col gap-3 py-3 md:flex-row md:items-center">
      <div className="flex min-w-0 flex-1 items-center gap-3">
        <Thumb title={need.name} width={36} />
        <Text textRole="body">{locked ? `Done — ${need.name} is sorted.` : need.line}</Text>
      </div>
      {locked ? (
        <StatusDot label="Done" tone="ok" />
      ) : (
        <div className="flex flex-wrap gap-2">
          {need.kind === "match" ? (
            <>
              <Action onPress={onYes} variant="secondary">
                Yes, that's it
              </Action>
              <Action onPress={() => go("unmatched")} variant="ghost">
                Pick a different title
              </Action>
            </>
          ) : null}
          {need.kind === "unknown" ? (
            <Action onPress={() => go("unmatched")} variant="secondary">
              Find it
            </Action>
          ) : null}
          {need.kind === "problem" ? (
            <Action onPress={() => go(need.opens ?? "item-series")} variant="secondary">
              See why
            </Action>
          ) : null}
        </div>
      )}
    </div>
  );
  return locked ? <LockIn onDone={onDone}>{body}</LockIn> : body;
};

const NeedsStrip = ({ scan, go }: { scan: ScanState; go: (s: Screen) => void }) => {
  const { answers, lock, done } = useAnswers();
  const [all, setAll] = useState(false);
  if (scan === "fresh") return null;
  const open = needsFor(scan).filter((n) => answers[n.id] !== "gone");
  const shown = all ? open : open.slice(0, 4);
  if (open.length === 0) {
    return (
      <Text textRole="body" tone="secondary">
        {scan === "idle" ? "All caught up — every title's ready to air." : "Nothing needs you."}
      </Text>
    );
  }
  return (
    <Surface borderColor="stateWarning" gap="inline" padding="control">
      <Text textRole="headline">Needs you ({open.filter((n) => answers[n.id] !== "locked").length})</Text>
      <div className="divide-border flex flex-col divide-y">
        {shown.map((n) => (
          <StripRow
            go={go}
            key={n.id}
            locked={answers[n.id] === "locked"}
            need={n}
            onDone={() => done(n.id)}
            onYes={() => lock(n.id)}
          />
        ))}
      </div>
      {open.length > 4 && !all ? (
        <div>
          <Action onPress={() => setAll(true)} variant="ghost">
            {`Show ${open.length - 4} more`}
          </Action>
        </div>
      ) : null}
    </Surface>
  );
};

// ── Full cards (Unmatched screen) ─────────────────────────────────────────────────────────────
const ReviewCard = ({
  need,
  onDone,
  primary,
  go,
}: {
  need: Need;
  onDone: () => void;
  primary: boolean;
  go: (s: Screen) => void;
}) => {
  const g = need.group;
  const best = g?.best;
  const problem = need.kind === "problem";
  const fileNames = g?.files ?? [];
  const [phase, setPhase] = useState<"open" | "added" | "ignored">("open");
  const [files, setFiles] = useState(false);
  const [other, setOther] = useState(false);
  const [q, setQ] = useState("");

  const headline = problem
    ? (need.headline ?? need.line)
    : best
      ? `Is this ${best.title} (${best.year})?`
      : "We're not sure what this is";
  const reason = problem
    ? (need.reason ?? "")
    : best
      ? reasonFor(best)
      : "The file name doesn't say what it is. Find it to pick the title yourself.";
  const many = fileNames.length > 1;

  const card = (
    <Surface gap="control" padding="section">
      <div className="flex flex-col gap-5 sm:flex-row">
        <div className="w-28 shrink-0 sm:w-36">
          <Poster big title={need.name} />
        </div>
        <div className="flex min-w-0 flex-1 flex-col gap-3">
          {phase === "open" ? (
            <>
              <Text textRole="title">{headline}</Text>
              <Text textRole="body" tone="secondary">
                {reason}
              </Text>
              {best ? (
                <Text textRole="compact" tone="muted">
                  {best.overview}
                </Text>
              ) : null}
              <div className="flex flex-wrap gap-2">
                {problem ? (
                  <Action onPress={() => go(need.opens ?? "item-series")} variant="secondary">
                    See why
                  </Action>
                ) : best ? (
                  <>
                    <Action
                      onPress={() => setPhase("added")}
                      tone={primary ? "primary" : "secondary"}
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
                {problem ? null : (
                  <Action onPress={() => setPhase("ignored")} variant="ghost">
                    Ignore this
                  </Action>
                )}
              </div>
              {other ? (
                <Surface gap="inline" padding="control">
                  <Field
                    label="Search for the title"
                    onChangeText={setQ}
                    placeholder="Type a movie or show name"
                    value={q}
                  />
                  {(g?.others ?? []).map((c) => (
                    <div className="flex items-center gap-3" key={`${c.title}${c.year}`}>
                      <Thumb title={c.title} width={28} />
                      <div className="min-w-0 flex-1">
                        <Text textRole="body">
                          {c.title} ({c.year})
                        </Text>
                      </div>
                      <Action onPress={() => setPhase("added")} variant="secondary">
                        Use this one
                      </Action>
                    </div>
                  ))}
                </Surface>
              ) : null}
              {fileNames.length > 0 ? (
                <Disclosure
                  expanded={files}
                  label={many ? `${fileNames.length} files` : "1 file"}
                  onExpandedChange={setFiles}
                >
                  <div className="flex flex-col gap-1 p-3">
                    {fileNames.map((f) => (
                      <Text key={f} textRole="compact" tone="secondary">
                        {f}
                      </Text>
                    ))}
                  </div>
                </Disclosure>
              ) : null}
            </>
          ) : (
            <div className="flex items-center gap-3 py-6">
              <StatusDot label="Done" tone="ok" />
              <Text textRole="title">
                {phase === "added" ? "Added to your library." : "Okay, we'll leave it out."}
              </Text>
            </div>
          )}
        </div>
      </div>
    </Surface>
  );
  return phase === "open" ? card : <LockIn onDone={onDone}>{card}</LockIn>;
};

const NeedsCards = ({ scan, go }: { scan: ScanState; go: (s: Screen) => void }) => {
  const { answers, done } = useAnswers();
  const needs = needsFor(scan).filter((n) => answers[n.id] !== "gone");
  return (
    <section aria-label="Needs you" className="flex flex-col gap-4">
      <Text textRole="headline">Needs you ({needs.length})</Text>
      {needs.length === 0 ? (
        <Text textRole="body" tone="secondary">
          Nothing needs you.
        </Text>
      ) : null}
      {needs.map((n, i) => (
        <ReviewCard go={go} key={n.id} need={n} onDone={() => done(n.id)} primary={i === 0} />
      ))}
    </section>
  );
};

export { NeedsCards, NeedsStrip, needsYouCount };
