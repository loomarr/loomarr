// PROTOTYPE — throwaway (#2151 step 2). Variant B right pane: the unmatched group's review card
// (decision 27) — folder, files, parse, best candidate with agreement ticks, other matches, actions.
import { Action, Badge, Disclosure, Text } from "@loomarr/design-system";
import { useState } from "react";
import type { Candidate, UnmatchedGroup } from "./prototype-data";
import { Art, Box, Meta, Panel, SectionLabel } from "./variant-b-parts";

const Tick = ({ ok, label }: { ok: boolean; label: string }) => (
  <Meta tone={ok ? "success" : "warning"}>{`${ok ? "✓" : "✗"} ${label}`}</Meta>
);

const CandidateBlock = ({ c, big }: { c: Candidate; big?: boolean }) => (
  <Box alignItems="flex-start" flexDirection="row" gap="control">
    <Art hue={c.hue} title={c.title} width={big ? 64 : 36} />
    <Box flex={1} gap={4} minWidth={0}>
      <Box alignItems="baseline" flexDirection="row" flexWrap="wrap" gap="inline">
        <Text textRole={big ? "label" : "body"}>{c.title}</Text>
        <Meta>{`${c.year} · ${c.kind} · tmdb ${c.tmdbId}`}</Meta>
      </Box>
      {big ? (
        <Text textRole="body" tone="secondary">
          {c.overview}
        </Text>
      ) : null}
      <Box flexDirection="row" flexWrap="wrap" gap="control">
        <Tick label="title" ok={c.agreement.title} />
        <Tick label="year" ok={c.agreement.year} />
        <Tick label="runtime" ok={c.agreement.runtime} />
      </Box>
    </Box>
  </Box>
);

const ReviewCard = ({
  group,
  decision,
  onDecide,
}: {
  group: UnmatchedGroup;
  decision?: string;
  onDecide: (label: string | null) => void;
}) => {
  const [filesOpen, setFilesOpen] = useState(false);
  const many = group.files.length > 3;
  return (
    <Box gap="section">
      <Box gap="inline">
        <Text textRole="title">{group.parsed}</Text>
        <Meta>{`${group.folder} · ${group.found}`}</Meta>
      </Box>

      <Box gap="inline">
        <SectionLabel>{`${group.files.length} ${group.files.length === 1 ? "file" : "files"}`}</SectionLabel>
        {many ? (
          <Disclosure
            description={`${group.files[0]} … ${group.files[group.files.length - 1]}`}
            expanded={filesOpen}
            label={`Show all ${group.files.length} file names`}
            onExpandedChange={setFilesOpen}
          >
            {group.files.map((f) => (
              <Meta key={f}>{f}</Meta>
            ))}
          </Disclosure>
        ) : (
          <Panel>
            {group.files.map((f) => (
              <div className="break-all" key={f}>
                <Text textRole="metadata" tone="primary">
                  {f}
                </Text>
              </div>
            ))}
          </Panel>
        )}
        <Meta>{`Parsed as · ${group.parsed}`}</Meta>
      </Box>

      {decision ? (
        <Panel alignItems="flex-start" gap="control" level="overlay">
          <Badge tone="success">{decision}</Badge>
          <Text textRole="body" tone="secondary">
            This is recorded as an explicit link. Links can be undone.
          </Text>
          <Action onPress={() => onDecide(null)} variant="outline">
            Undo
          </Action>
        </Panel>
      ) : (
        <>
          {group.best ? (
            <Box gap="inline">
              <SectionLabel>Best candidate</SectionLabel>
              <Panel gap="control">
                <CandidateBlock big c={group.best} />
              </Panel>
            </Box>
          ) : (
            <Panel gap="control">
              <Text textRole="label">No candidate</Text>
              <Text textRole="body" tone="secondary">
                Nothing in this name matched a title. Search for it, or mark it as not a title.
              </Text>
            </Panel>
          )}

          {group.others.length ? (
            <Box gap="inline">
              <SectionLabel>Other matches</SectionLabel>
              {group.others.map((c) => (
                <Panel gap="control" key={c.tmdbId}>
                  <CandidateBlock c={c} />
                  <Box flexDirection="row">
                    <Action onPress={() => onDecide(`Linked to ${c.title} (${c.year})`)} variant="outline">
                      Use this one
                    </Action>
                  </Box>
                </Panel>
              ))}
            </Box>
          ) : null}

          <Box flexDirection="row" flexWrap="wrap" gap="inline">
            {group.best ? (
              <Action onPress={() => onDecide(`Confirmed ${group.best?.title} (${group.best?.year})`)}>
                Confirm
              </Action>
            ) : null}
            <Action icon="search" variant="outline">
              Search…
            </Action>
            <Action onPress={() => onDecide("Marked not a title")} variant="outline">
              Not a title
            </Action>
          </Box>
        </>
      )}
    </Box>
  );
};

export { ReviewCard };
