// PROTOTYPE — throwaway (#2151 step 2). Variant C · Unmatched as a focused review mode: one group
// at a time, a big centred card, progress, Previous/Next and keyboard shortcuts.
import { Action, Badge, Disclosure, ProgressTrack, Surface, Text } from "@loomarr/design-system";
import { useEffect, useState } from "react";
import { type Candidate, UNMATCHED } from "./prototype-data";
import { Art, SectionLabel } from "./variant-c-parts";

type Decision = "confirmed" | "search" | "not-a-title";

const Tick = ({ ok, label }: { ok: boolean; label: string }) => (
  <Badge tone={ok ? "success" : "caution"}>
    {ok ? "✓" : "≠"} {label}
  </Badge>
);

const CandidateCard = ({ c, big }: { c: Candidate; big?: boolean }) => (
  <div className="flex items-start gap-3">
    <Art hue={c.hue} title={c.title} width={big ? 72 : 36} />
    <div className="flex min-w-0 flex-1 flex-col gap-1">
      <Text textRole={big ? "headline" : "label"}>
        {c.title}{" "}
        <Text textRole="data" tone="secondary">
          {c.year}
        </Text>
      </Text>
      <Text textRole="metadata" tone="muted">
        {c.kind} · TMDB {c.tmdbId}
      </Text>
      {big ? (
        <Text textRole="body" tone="secondary">
          {c.overview}
        </Text>
      ) : null}
      {big ? (
        <div className="flex flex-wrap gap-2 pt-1">
          <Tick label="title" ok={c.agreement.title} />
          <Tick label="year" ok={c.agreement.year} />
          <Tick label="runtime" ok={c.agreement.runtime} />
        </div>
      ) : null}
    </div>
  </div>
);

const ReviewPage = () => {
  const [index, setIndex] = useState(0);
  const [decisions, setDecisions] = useState<Record<string, Decision>>({});
  const [filesOpen, setFilesOpen] = useState(false);
  const group = UNMATCHED[index];
  const total = UNMATCHED.length;
  const decided = Object.keys(decisions).length;

  const decide = (d: Decision) => {
    if (!group) return;
    if (d === "confirmed" && !group.best) return;
    setDecisions((prev) => ({ ...prev, [group.id]: d }));
    setFilesOpen(false);
    if (d !== "search") setIndex((i) => Math.min(i + 1, total - 1));
  };
  const step = (by: number) => {
    setIndex((i) => Math.max(0, Math.min(total - 1, i + by)));
    setFilesOpen(false);
  };

  // Keyboard hints are real: C confirm · S search · N not a title. Arrow keys belong to the prototype switcher.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      if (el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA")) return;
      const k = e.key.toLowerCase();
      if (k === "c") decide("confirmed");
      else if (k === "s") decide("search");
      else if (k === "n") decide("not-a-title");
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  if (!group) return null;
  const decision = decisions[group.id];
  const shownFiles = filesOpen ? group.files : group.files.slice(0, 2);

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <div className="flex items-center justify-between gap-3">
        <Text textRole="label">
          {index + 1} of {total}
        </Text>
        <Text textRole="metadata" tone="secondary">
          {decided} decided · {group.found}
        </Text>
      </div>
      <ProgressTrack label="Review progress" percent={Math.round((decided / total) * 100)} />

      <Surface gap="section" padding="section">
        <div className="flex flex-col gap-1">
          <SectionLabel>Folder</SectionLabel>
          <Text textRole="data">{group.folder}</Text>
        </div>
        <div className="flex flex-col gap-1">
          <SectionLabel>{group.files.length === 1 ? "File" : `${group.files.length} files`}</SectionLabel>
          {shownFiles.map((f) => (
            <Text key={f} textRole="data" tone="secondary">
              {f}
            </Text>
          ))}
          {group.files.length > 2 ? (
            <Disclosure
              expanded={filesOpen}
              label={filesOpen ? "Show fewer files" : `Show all ${group.files.length} files`}
              onExpandedChange={setFilesOpen}
            >
              <span />
            </Disclosure>
          ) : null}
        </div>
        <div className="flex flex-col gap-1">
          <SectionLabel>Parsed as</SectionLabel>
          <Text textRole="body">{group.parsed}</Text>
        </div>

        <div className="flex flex-col gap-2">
          <SectionLabel>Best candidate</SectionLabel>
          {group.best ? (
            <Surface level="elevated" padding="control">
              <CandidateCard big c={group.best} />
            </Surface>
          ) : (
            <Surface level="elevated" padding="control">
              <Text textRole="body" tone="secondary">
                No candidate. Nothing recognisable in the name, so search by hand or mark it as not a title.
              </Text>
            </Surface>
          )}
        </div>

        {group.others.length > 0 ? (
          <div className="flex flex-col gap-2">
            <SectionLabel>Other matches</SectionLabel>
            {group.others.map((c) => (
              <CandidateCard c={c} key={c.tmdbId} />
            ))}
          </div>
        ) : null}

        <div className="flex flex-wrap items-center gap-2">
          {group.best ? (
            <Action onPress={() => decide("confirmed")} tone="primary">
              Confirm · C
            </Action>
          ) : null}
          <Action onPress={() => decide("search")} variant="secondary">
            Search… · S
          </Action>
          <Action onPress={() => decide("not-a-title")} variant="outline">
            Not a title · N
          </Action>
          {decision ? (
            <Badge tone={decision === "confirmed" ? "success" : "info"}>
              {decision === "confirmed" ? "confirmed" : decision === "search" ? "searching" : "not a title"}
            </Badge>
          ) : null}
        </div>
      </Surface>

      <div className="flex items-center justify-between gap-2">
        <Action disabled={index === 0} icon="previous" onPress={() => step(-1)} variant="ghost">
          Previous
        </Action>
        <Text textRole="metadata" tone="muted">
          C confirm · S search · N not a title{" "}
        </Text>
        <Action disabled={index === total - 1} onPress={() => step(1)} variant="ghost">
          Next
        </Action>
      </div>
    </div>
  );
};

export { ReviewPage };
