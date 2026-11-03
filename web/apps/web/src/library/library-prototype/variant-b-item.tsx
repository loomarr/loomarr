// PROTOTYPE — throwaway (#2151 step 2). Variant B right pane: the item view (decision 25) —
// header, fields with source chips (+ provenance), files, seasons, used-by, actions.
import { Action, Badge, Disclosure, Text } from "@loomarr/design-system";
import { useState } from "react";
import type { Episode, FactsState, FieldValue, ItemDetail, ItemRow, SourceFile } from "./prototype-data";
import { Art, Box, FactsMark, IdentityChip, Meta, Panel, SectionLabel, SourceChip } from "./variant-b-parts";

const FieldRow = ({ field, open, onToggle }: { field: FieldValue; open: boolean; onToggle: () => void }) => (
  <Box gap="inline">
    <Box alignItems="flex-start" flexDirection="row" flexWrap="wrap" gap="inline">
      <Box width={96}>
        <Meta>{field.label}</Meta>
      </Box>
      <Box flex={1} minWidth={160}>
        <Text textRole="body">{field.value}</Text>
      </Box>
      <SourceChip active={open} onPress={onToggle} source={field.source} />
    </Box>
    {open ? (
      <Panel level="overlay">
        <SectionLabel>{`Why ${field.value.length > 24 ? field.label : field.value}?`}</SectionLabel>
        {field.candidates ? (
          field.candidates.map((c) => (
            <Box alignItems="flex-start" flexDirection="row" gap="inline" key={c.source}>
              <Box width={64}>
                <Text strong={c.source === field.source} textRole="metadata">
                  {c.source}
                </Text>
              </Box>
              <Box flex={1}>
                <Text textRole="metadata" tone={c.source === field.source ? "primary" : "secondary"}>
                  {c.value}
                </Text>
              </Box>
              {c.source === field.source ? <Badge tone="success">won</Badge> : null}
            </Box>
          ))
        ) : (
          <Meta>{`Only ${field.source} provides this field.`}</Meta>
        )}
        {field.rule ? <Meta>{`Rule · ${field.rule}`}</Meta> : null}
      </Panel>
    ) : null}
  </Box>
);

const FileCard = ({ file, facts, offline }: { file: SourceFile; facts: FactsState; offline: boolean }) => (
  <Panel level={file.stray ? "canvas" : "raised"}>
    <Box alignItems="center" flexDirection="row" flexWrap="wrap" gap="inline">
      {file.stray ? <Badge tone="warning">stray copy</Badge> : <Badge tone="success">plays</Badge>}
      {file.edition ? <Badge tone="neutral">{file.edition}</Badge> : null}
      {offline ? <Badge tone="danger">offline</Badge> : null}
      <FactsMark facts={facts} />
    </Box>
    <div className="break-all">
      <Text textRole="metadata" tone="primary">
        {file.path}
      </Text>
    </div>
    {file.stray ? <Meta tone="warning">Never plays — the arr's current file plays instead.</Meta> : null}
    <Meta>{`${file.size} · ${file.container} · ${file.duration} · ${file.chapters} chapters`}</Meta>
    <Meta>{`Video · ${file.video}`}</Meta>
    <Meta>{`Audio · ${file.audio.join("; ")}`}</Meta>
    <Meta>{`Subs · ${file.subtitles.length ? file.subtitles.join(", ") : "none"}`}</Meta>
    <Meta tone="muted">{`Revision · ${file.revision}`}</Meta>
  </Panel>
);

const EpisodeRow = ({ ep }: { ep: Episode }) => (
  <Box alignItems="center" flexDirection="row" flexWrap="wrap" gap="inline">
    <Box width={60}>
      <Text textRole="metadata" tone="secondary">
        {ep.code}
      </Text>
    </Box>
    <Box flex={1} minWidth={140}>
      <Text numberOfLines={1} textRole="body">
        {ep.title}
      </Text>
    </Box>
    {ep.special ? <Badge tone="info">{ep.special}</Badge> : null}
    <Meta>{ep.runtime}</Meta>
    <Meta tone={ep.ratingInherited ? "muted" : "secondary"}>
      {ep.ratingInherited ? `${ep.rating} ↑ series` : `${ep.rating} own`}
    </Meta>
    <FactsMark facts={ep.facts} />
  </Box>
);

const ItemView = ({
  detail,
  row,
  openField,
  onToggleField,
  offline,
}: {
  detail: ItemDetail;
  row: ItemRow;
  openField: string | null;
  onToggleField: (label: string) => void;
  offline: boolean;
}) => {
  const [seasonOpen, setSeasonOpen] = useState(true);
  const [fixOpen, setFixOpen] = useState(false);
  const [queued, setQueued] = useState<string | null>(null);
  const arr = detail.identity === "Sonarr" || detail.identity === "Radarr" ? detail.identity : null;
  const pendingFacts = row.facts === "waiting" || row.facts === "reading";

  return (
    <Box gap="section">
      <Box alignItems="flex-start" flexDirection="row" gap="control">
        <Art hue={detail.hue} title={detail.title} width={72} />
        <Box flex={1} gap="inline" minWidth={0}>
          <Text textRole="title">{detail.title}</Text>
          <Box alignItems="center" flexDirection="row" flexWrap="wrap" gap="inline">
            <Meta>{`${detail.year} · ${detail.kind}${detail.status ? ` · ${detail.status}` : ""}`}</Meta>
            <Badge tone="neutral">{row.certification}</Badge>
            <IdentityChip source={detail.identity} />
            <FactsMark detail={row.factsDetail} facts={row.facts} />
            {offline ? <Badge tone="danger">offline</Badge> : null}
          </Box>
          <Meta tone="muted">{detail.ids.map((i) => `${i.namespace} ${i.value}`).join(" · ")}</Meta>
        </Box>
      </Box>

      <Box flexDirection="row" flexWrap="wrap" gap="inline">
        <Action onPress={() => setQueued("Metadata refresh queued")} variant="outline">
          Refresh metadata
        </Action>
        <Action onPress={() => setQueued("Re-reading file headers")} variant="outline">
          Re-read files
        </Action>
        <Action onPress={() => setFixOpen((o) => !o)} selected={fixOpen} variant="outline">
          Fix match
        </Action>
      </Box>
      {queued ? <Meta tone="success">{`✓ ${queued}`}</Meta> : null}
      {fixOpen ? (
        <Panel alignItems="flex-start" gap="control" level="overlay">
          {arr ? (
            <>
              <Text textRole="label">{`${arr} owns this identity`}</Text>
              <Text textRole="body" tone="secondary">
                {`This match comes from ${arr}. Change it there and Loomarr follows on the next webhook.`}
              </Text>
              <Action onPress={() => setFixOpen(false)}>{`Open in ${arr}`}</Action>
            </>
          ) : (
            <>
              <Text textRole="label">Pick the right title</Text>
              <Text textRole="body" tone="secondary">
                Choosing a match writes an explicit link. Links can be undone.
              </Text>
              <Action onPress={() => setFixOpen(false)}>Search…</Action>
            </>
          )}
        </Panel>
      ) : null}

      <Box gap="inline">
        <SectionLabel>Metadata · source and why it won</SectionLabel>
        <Panel gap="control">
          {detail.fields.map((f) => (
            <FieldRow
              field={f}
              key={f.label}
              onToggle={() => onToggleField(f.label)}
              open={openField === f.label}
            />
          ))}
        </Panel>
      </Box>

      {detail.seasons ? (
        <Box gap="inline">
          <SectionLabel>Seasons</SectionLabel>
          {detail.seasons.map((s) =>
            s.number === 4 && detail.openSeason ? (
              <Disclosure
                description={`${s.episodes} episodes · ${s.read}/${s.episodes} read`}
                expanded={seasonOpen}
                key={s.number}
                label={s.label}
                onExpandedChange={setSeasonOpen}
              >
                {detail.openSeason.map((ep) => (
                  <EpisodeRow ep={ep} key={ep.code} />
                ))}
              </Disclosure>
            ) : (
              <Box
                alignItems="center"
                flexDirection="row"
                gap="control"
                key={s.number}
                paddingHorizontal="control"
                paddingVertical={4}
              >
                <Box flex={1}>
                  <Text textRole="body">{s.label}</Text>
                </Box>
                <Meta>{`${s.episodes} episodes · ${s.read}/${s.episodes} read`}</Meta>
              </Box>
            ),
          )}
        </Box>
      ) : null}

      {detail.files ? (
        <Box gap="inline">
          <SectionLabel>
            {detail.kind === "Series" ? "Files · Media Sources (latest episode)" : "Files · Media Sources"}
          </SectionLabel>
          {detail.files.map((f) => (
            <FileCard facts={pendingFacts ? row.facts : f.facts} file={f} key={f.path} offline={offline} />
          ))}
        </Box>
      ) : null}

      <Box gap="inline">
        <SectionLabel>Used by</SectionLabel>
        {detail.usedBy.map((u) => (
          <Box alignItems="center" flexDirection="row" flexWrap="wrap" gap="inline" key={u.channel}>
            <Badge tone="info">{u.channel}</Badge>
            <Meta>{u.how}</Meta>
          </Box>
        ))}
      </Box>

      <Box gap="inline">
        <SectionLabel>Artwork</SectionLabel>
        <Box flexDirection="row" flexWrap="wrap" gap="control">
          {detail.artwork.map((a) => (
            <Box alignItems="center" flexDirection="row" gap="inline" key={a.role}>
              <Meta>{a.role}</Meta>
              <SourceChip source={a.source} />
            </Box>
          ))}
        </Box>
      </Box>
    </Box>
  );
};

// Items whose full view isn't mocked: header and actions only.
const ItemStub = ({ row, offline }: { row: ItemRow; offline: boolean }) => {
  const arr = row.identity === "Sonarr" || row.identity === "Radarr" ? row.identity : null;
  return (
    <Box gap="section">
      <Box alignItems="flex-start" flexDirection="row" gap="control">
        <Art hue={row.hue} title={row.title} width={72} />
        <Box flex={1} gap="inline" minWidth={0}>
          <Text textRole="title">{row.title}</Text>
          <Box alignItems="center" flexDirection="row" flexWrap="wrap" gap="inline">
            <Meta>{`${row.year} · ${row.kind}`}</Meta>
            <Badge tone="neutral">{row.certification}</Badge>
            <IdentityChip source={row.identity} />
            <FactsMark detail={row.factsDetail} facts={row.facts} />
            {offline ? <Badge tone="danger">offline</Badge> : null}
          </Box>
          {row.problems?.map((p) => (
            <Meta key={p} tone="warning">{`! ${p}`}</Meta>
          ))}
        </Box>
      </Box>
      <Box flexDirection="row" flexWrap="wrap" gap="inline">
        <Action variant="outline">Refresh metadata</Action>
        <Action variant="outline">Re-read files</Action>
        <Action variant="outline">{arr ? `Open in ${arr}` : "Fix match"}</Action>
      </Box>
      <Meta tone="muted">The full item view is mocked for The West Wing and Fargo.</Meta>
    </Box>
  );
};

export { ItemStub, ItemView };
