// PROTOTYPE — throwaway (#2151 step 2). Variant A: Unmatched review cards (decision 27).
import { Action, Badge, Disclosure, Surface, Text } from "@loomarr/design-system";
import { useState } from "react";
import type { Candidate, UnmatchedGroup } from "./prototype-data";
import { UNMATCHED } from "./prototype-data";
import { Art } from "./variant-a-parts";

const Ticks = ({ c }: { c: Candidate }) => (
  <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" flexWrap="wrap" gap={6}>
    <Badge tone={c.agreement.title ? "success" : "warning"}>
      {c.agreement.title ? "✓ title" : "✗ title"}
    </Badge>
    <Badge tone={c.agreement.year ? "success" : "warning"}>{c.agreement.year ? "✓ year" : "✗ year"}</Badge>
    <Badge tone={c.agreement.runtime ? "success" : "warning"}>
      {c.agreement.runtime ? "✓ runtime" : "✗ runtime"}
    </Badge>
  </Surface>
);

const CandidateRow = ({ c, best }: { c: Candidate; best?: boolean }) => (
  <Surface
    backgroundColor={best ? "surfaceElevated" : "transparent"}
    borderWidth={best ? 1 : 0}
    flexDirection="row"
    gap="control"
    padding={best ? "control" : 0}
  >
    <Art hue={c.hue} width={best ? 48 : 32} />
    <Surface backgroundColor="transparent" borderWidth={0} flex={1} gap={4} minWidth={0}>
      <Surface
        alignItems="center"
        backgroundColor="transparent"
        borderWidth={0}
        flexDirection="row"
        flexWrap="wrap"
        gap="inline"
      >
        {best ? <Badge tone="signal">best match</Badge> : null}
        <Text textRole="label" tone="primary">{`${c.title} (${c.year})`}</Text>
        <Badge>{c.kind}</Badge>
        <Text textRole="metadata">{`tmdb:${c.tmdbId}`}</Text>
      </Surface>
      <Text numberOfLines={best ? 3 : 1} textRole="compact">
        {c.overview}
      </Text>
      <Ticks c={c} />
    </Surface>
  </Surface>
);

type Decision = "confirmed" | "dismissed";

const Card = ({ group }: { group: UnmatchedGroup }) => {
  const [decision, setDecision] = useState<Decision | null>(null);
  const [filesOpen, setFilesOpen] = useState(false);
  const [othersOpen, setOthersOpen] = useState(false);
  const many = group.files.length > 3;

  if (decision) {
    return (
      <Surface alignItems="center" flexDirection="row" gap="control" padding="control">
        <Badge tone={decision === "confirmed" ? "success" : "neutral"}>
          {decision === "confirmed" ? "linked" : "not a title"}
        </Badge>
        <Surface backgroundColor="transparent" borderWidth={0} flex={1}>
          <Text numberOfLines={1} textRole="compact" tone="primary">
            {group.folder}
          </Text>
        </Surface>
        <Action onPress={() => setDecision(null)} variant="link">
          Undo
        </Action>
      </Surface>
    );
  }

  return (
    <Surface gap="control" padding="control">
      <Surface backgroundColor="transparent" borderWidth={0} gap={2}>
        <Text textRole="data" tone="primary" wordWrap="break-word">
          {group.folder}
        </Text>
        <Text textRole="metadata">{`${group.files.length} ${group.files.length === 1 ? "file" : "files"} · ${group.found}`}</Text>
      </Surface>

      {many ? (
        <Disclosure
          expanded={filesOpen}
          label={`${group.files.length} files`}
          onExpandedChange={setFilesOpen}
        >
          <Surface backgroundColor="transparent" borderWidth={0} gap={2} padding="control">
            {group.files.map((f) => (
              <Text key={f} textRole="metadata" tone="primary">
                {f}
              </Text>
            ))}
          </Surface>
        </Disclosure>
      ) : (
        <Surface backgroundColor="transparent" borderWidth={0} gap={2}>
          {group.files.map((f) => (
            <Text key={f} textRole="metadata" tone="primary" wordWrap="break-word">
              {f}
            </Text>
          ))}
        </Surface>
      )}

      <Text textRole="compact">{`Parsed as · ${group.parsed}`}</Text>

      {group.best ? (
        <CandidateRow best c={group.best} />
      ) : (
        <Text textRole="compact" tone="warning">
          No candidate — search TMDB by hand, or mark it as not a title.
        </Text>
      )}

      {group.others.length > 0 ? (
        <Disclosure
          expanded={othersOpen}
          label={`Other matches (${group.others.length})`}
          onExpandedChange={setOthersOpen}
        >
          <Surface backgroundColor="transparent" borderWidth={0} gap="control" padding="control">
            {group.others.map((c) => (
              <CandidateRow c={c} key={c.tmdbId} />
            ))}
          </Surface>
        </Disclosure>
      ) : null}

      <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" flexWrap="wrap" gap="inline">
        {group.best ? (
          <Action icon="success" onPress={() => setDecision("confirmed")} tone="primary">
            Confirm
          </Action>
        ) : null}
        <Action icon="search" tone="secondary">
          Search…
        </Action>
        <Action onPress={() => setDecision("dismissed")} variant="outline">
          Not a title
        </Action>
      </Surface>
    </Surface>
  );
};

const UnmatchedStack = ({ count }: { count: number }) =>
  count === 0 ? (
    <Surface gap="inline" padding="section">
      <Text textRole="title">Nothing to review</Text>
      <Text textRole="body" tone="secondary">
        Files Loomarr cannot ground in Sonarr, Radarr, a name or an NFO appear here with a best candidate.
      </Text>
    </Surface>
  ) : (
    <Surface backgroundColor="transparent" borderWidth={0} gap="control">
      {UNMATCHED.map((g) => (
        <Card group={g} key={g.id} />
      ))}
    </Surface>
  );

export { UnmatchedStack };
